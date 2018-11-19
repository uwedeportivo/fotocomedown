# fotocomedown

[![Build Status](https://travis-ci.com/uwedeportivo/fotocomedown.svg?branch=master)](https://travis-ci.com/uwedeportivo/fotocomedown)

fotocomedown is a command line tool to download all your photos from [fotocommunity.de](https://www.fotocommunity.de).

## Warning

This script scrapes the HTML UI of fotocommunity.de. It is by nature a brittle situation that can break whenever fotocommunity changes the UI. I can try to keep up, but no promises. PRs are always welcome.

## Motivation

My father is a pretty good amateur photographer and has been a member of [fotocommunity.de](https://www.fotocommunity.de) for a long time. He has accumulated a large collection of photos on the site. Unfortunately he has not organized this collection locally on his computer and photos are scattered over many hard disks and directories. fotocommunity.de does not offer the functionality of downloading all of ones photos with one action. One can go to each image and click on the "Original Image" link and then download it, but that is super tedious. So in the spirit of data portability I made this simple script.

## Installation

* Install [Go](http://golang.org/doc/install):

Visit [http://golang.org/doc/install](http://golang.org/doc/install) and follow instructions for your platform.

Edit your _~/.bash_profile_ file adding the following lines:

```
export GOPATH=$HOME/go
export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH
```

Reload your profile file:

```
. ~/.bash_profile
```

* Install fotocomedown

```
go get github.com/uwedeportivo/fotocomedown
```

## Usage

Open a terminal and use fotocomedown. You will be prompted for the password of the user you specify.

```
NAME:
   fotocomedown - command line app downloads all your photos from fotocommunity.de

USAGE:
   fotocomedown [global options] command [command options] [arguments...]

VERSION:
   1.0

DESCRIPTION:
   Download all your photos from fotocommunity.de.

AUTHOR:
   Uwe Hoffmann

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --out DIR, -o DIR     Output DIR where images are downloaded to
   --user USER, -u USER  fotocommunity.de USER for whom images are downloaded
   --help, -h            show help
   --version, -v         print the version

COPYRIGHT:
   Uwe Hoffmann (c) 2018
```



