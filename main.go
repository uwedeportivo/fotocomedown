package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/howeyc/gopass"
	"github.com/urfave/cli"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/cheggaaa/pb.v1"
)

const (
	numWorkers = 20
	topUrl     = "https://www.fotocommunity.de"
	loginUrl   = "https://www.fotocommunity.de/login"
	userPhotos = "https://www.fotocommunity.de/user_photos"
)

var userIdRegex = regexp.MustCompile(`\[fc-user:(\d+)\]`)
var paginationRegex = regexp.MustCompile(`Seite\s+(\d+)\s+von\s+(\d+)`)

func login(user, password string) (*http.Client, int, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, -1, err
	}

	client := &http.Client{
		Jar: jar,
	}

	_, err = client.Get(loginUrl)
	if err != nil {
		return nil, -1, err
	}

	values := make(url.Values)
	values.Set("login[login]", user)
	values.Set("login[pass]", password)
	values.Set("login[vorname]", "")
	values.Set("signup", "Einloggen")

	data := values.Encode()

	req, err := http.NewRequest("POST", loginUrl, strings.NewReader(data))
	if err != nil {
		return nil, -1, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/12.0.1 Safari/605.1.15")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, -1, err
	}
	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, -1, err
	}
	page := string(bs)

	ms := userIdRegex.FindStringSubmatch(page)
	if len(ms) != 2 {
		return nil, -1, fmt.Errorf("failed to find user id after login")
	}

	userId, err := strconv.ParseInt(ms[1], 10, 64)
	if err != nil {
		return nil, -1, err
	}

	return client, int(userId), nil
}

type photoInfo struct {
	counter int
	filename string
	title string
	pageUrl string
	originalUrl string
	originalUrlPrefix string
	dataId int
	client *http.Client
}

func extractPhotoInfo(anchor *html.Node, counter int, outDirPath string) (*photoInfo, error) {
	link := scrape.Attr(anchor, "href")
	dataIdStr := scrape.Attr(anchor, "data-id")

	dataId, err := strconv.ParseInt(dataIdStr, 10, 64)
	if err != nil {
		return nil, err
	}

	if anchor.FirstChild == nil {
		return nil, fmt.Errorf("child node on anchor expected")
	}

	title := scrape.Attr(anchor.FirstChild, "alt")

	dataSrcUrl, err := url.Parse(scrape.Attr(anchor.FirstChild, "data-src"))
	if err != nil {
		return nil, err
	}

	p := path.Base(dataSrcUrl.Path)
	index := len(p) - 41
	p = p[:index]
	filename := fmt.Sprintf("%04d-%s.jpg", counter, p)
	dataSrcUrl.RawQuery = ""

	originalUrlPrefix := dataSrcUrl.String()

	return &photoInfo{
		counter:counter,
		pageUrl:topUrl + link,
		dataId: int(dataId),
		title: title,
		filename: filepath.Join(outDirPath, filename),
		originalUrlPrefix:originalUrlPrefix,
	}, nil
}

func extractNextPage(div *html.Node) (int, error) {
	buf := new(bytes.Buffer)
	err := html.Render(buf, div)
	if err != nil {
		return -1, err
	}

	snippet := buf.String()

	ms := paginationRegex.FindStringSubmatch(snippet)
	if len(ms) != 3 {
		return -1, fmt.Errorf("failed to process pagination info")
	}

	currentPage, err := strconv.ParseInt(ms[1], 10, 64)
	if err != nil {
		return -1, err
	}
	totalPages, err := strconv.ParseInt(ms[2], 10, 64)
	if err != nil {
		return -1, err
	}

	if currentPage < totalPages {
		return int(currentPage+1), nil
	}
	return -1, nil
}

func photosPage(client *http.Client, url string, counter int, outDirPath string) ([]*photoInfo, int, int, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, -1, -1, err
	}
	defer resp.Body.Close()

	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, -1, -1, err
	}

	photosAnchorMatcher := func(n *html.Node) bool {
		return n.DataAtom == atom.A && scrape.Attr(n, "class") == "fcx-detail-link fcx-show-detail"
	}

	anchors := scrape.FindAll(root, photosAnchorMatcher)

	pis := make([]*photoInfo, 0, len(anchors))
	for i, anchor := range anchors {
		pi, err := extractPhotoInfo(anchor, i + counter, outDirPath)
		if err != nil {
			return nil, -1, -1, err
		}
		pi.client = client
		pis = append(pis, pi)
	}

	// pagination

	paginationMatcher := func(n *html.Node) bool {
		return n.DataAtom == atom.Div && scrape.Attr(n, "class") == "fcx-pagination text-center text-right-md"
	}

	paginationDivs := scrape.FindAll(root, paginationMatcher)

	if len(paginationDivs) == 0 {
		return nil, -1, -1, fmt.Errorf("no pagination information available")
	}

	nextPageIndex, err := extractNextPage(paginationDivs[0])
	if err != nil {
		return nil, -1, -1, err
	}

	return pis, nextPageIndex, counter + len(pis), nil
}

func photoPage(pi *photoInfo) error {
	resp, err := pi.client.Get(pi.pageUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	root, err := html.Parse(resp.Body)
	if err != nil {
		return err
	}

	origAnchorMatcher := func(n *html.Node) bool {
		if n.DataAtom != atom.A {
			return false
		}

		u := scrape.Attr(n, "href")
		return strings.HasPrefix(u, pi.originalUrlPrefix)
	}

	anchors := scrape.FindAll(root, origAnchorMatcher)

	if len(anchors) == 0 {
		return fmt.Errorf("no original image url found for %s", pi.pageUrl)
	}

	pi.originalUrl = scrape.Attr(anchors[0], "href")

	return nil
}

func down(pi *photoInfo) error {
	filename := pi.filename

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := pi.client.Get(pi.originalUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func worker(c chan *photoInfo, wg *sync.WaitGroup, bar *pb.ProgressBar) {
	for pi := range c {
		err := photoPage(pi)
		if err != nil {
			fmt.Printf("error getting original image url %s: %v", pi.pageUrl, err)
		}

		err = down(pi)
		if err != nil {
			fmt.Printf("error downloading %s: %v", pi.originalUrl, err)
		}
		bar.Increment()
	}
	wg.Done()
}

func setupOutDir(dirName string) (string, error) {
	result := dirName
	if dirName == "" {
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		result = dir
	}

	absPath, err := filepath.Abs(result)
	if err != nil {
		return "", err
	}

	result = absPath

	fi, err := os.Lstat(result)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(result, 0777)
			if err != nil {
				return "", err
			}

			fi, err = os.Lstat(result)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	if !fi.IsDir() {
		return "", fmt.Errorf("%s not a dir", result)
	}

	return result, nil
}

func fotoComeDown(ctxt *cli.Context) error {
	user := ctxt.String("user")
	if user == "" {
		fmt.Printf("--user flag required\n\n")
		cli.ShowAppHelpAndExit(ctxt, 1)
	}

	outDirPath, err := setupOutDir(ctxt.String("out"))
	if err != nil {
		return err
	}

	fmt.Printf("Please enter password for user %s:\n", user)
	pass, err := gopass.GetPasswd()
	if err != nil {
		return err
	}

	client, userId, err := login(user, string(pass))
	if err != nil {
		return err
	}

	fmt.Printf("downloading all photos for user %s into output directory %s\n\n", user, outDirPath)

	var allPis []*photoInfo

	nextPageIndex := 1

	counter := 0

	for nextPageIndex != -1 {
		photosUrl := fmt.Sprintf("%s/%d?sort=new&page=%d", userPhotos, userId, nextPageIndex)

		fmt.Printf("fetching image urls from page %d\n", nextPageIndex)
		spn := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
		spn.Start()

		pis, i, j, err := photosPage(client, photosUrl, counter, outDirPath)
		if err != nil {
			return err
		}

		allPis = append(allPis, pis...)
		nextPageIndex = i
		counter = j

		spn.Stop()
	}

	if len(allPis) == 0 {
		return fmt.Errorf("no images found")
	}

	fmt.Printf("fetched %d image urls, downloading images\n", len(allPis))

	bar := pb.StartNew(len(allPis))

	var wg sync.WaitGroup

	c := make(chan *photoInfo)

	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go worker(c, &wg, bar)
	}

	for _, pi := range allPis {
		c <- pi
	}

	close(c)
	wg.Wait()

	bar.Finish()

	fmt.Println("Done.")
	return nil
}

func main() {
	app := cli.NewApp()

	app.Name = "fotocomedown"
	app.Version = "1.0"
	app.Description = "Download all your photos from fotocommunity.de."
	app.Author = "Uwe Hoffmann"
	app.Copyright = "Uwe Hoffmann (c) 2018"
	app.HelpName = "fotocomedown"
	app.Usage = "command line app downloads all your photos from fotocommunity.de"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "out, o",
			Usage: "Output `DIR` where images are downloaded to",
		},
		cli.StringFlag{
			Name:  "user, u",
			Usage: "fotocommunity.de `USER` for whom images are downloaded",
		},
	}

	app.Action = fotoComeDown

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
