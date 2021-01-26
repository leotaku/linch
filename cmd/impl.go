package cmd

import (
	"bufio"
	"errors"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"

	http "github.com/valyala/fasthttp"
)

var regexpGuessOne = regexp.MustCompile(`(https?):\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,4}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)

type Action struct {
	Original *url.URL
	Redir    *url.URL
	Status   int
	Err      error
}

func extractLinksForPaths(s *bufio.Scanner, send chan string) {
	for s.Scan() {
		extractLinksForPath(s.Text(), send)
	}
	close(send)
}

func extractLinksForPath(path string, send chan string) {
	file, err := openValidFile(path)
	defer file.Close()
	if err != nil {
		return
	}

	buf := bufio.NewScanner(file)
	for buf.Scan() {
		matches := regexpGuessOne.FindAllString(buf.Text(), -1)
		for _, rawurl := range matches {
			send <- rawurl
		}
	}
}

func startLinkHandler() (chan string, chan Action) {
	links := make(chan string, 1000)
	rsps := make(chan Action, 100)

	go func() {
		wg := new(sync.WaitGroup)
		for i := 0; i < limitArg; i++ {
			wg.Add(1)
			go func(wg *sync.WaitGroup) {
				for link := range links {
					handleRawLink(link, rsps)
					time.Sleep(waitArg)
				}
				wg.Done()
			}(wg)
		}
		wg.Wait()
		close(rsps)
	}()

	return links, rsps
}

var syncmap sync.Map

func handleRawLink(rawurl string, rsps chan Action) {
	_, loaded := syncmap.LoadOrStore(rawurl, nil)
	if loaded {
		return
	}

	url, err := url.Parse(rawurl)
	if err != nil {
		rsps <- Action{Err: err}
	} else {
		rsps <- handleLink(*url)
	}
}

func handleLink(link url.URL) Action {
	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.Header.SetMethod("HEAD")
	req.SetRequestURI(link.String())
	err := http.DoTimeout(req, resp, timeoutArg)
	status := resp.StatusCode()

	switch {
	case err != nil:
		return Action{
			Original: &link,
			Err:      err,
		}
	case status >= 300 && status < 400:
		redir := string(resp.Header.Peek("Location"))
		redurl, err := url.Parse(redir)
		if len(redir) == 0 {
			err = errors.New("missing location in redirection")
		} else {
			if len(redurl.Host) == 0 {
				redurl.Host = link.Host
			}
			if len(redurl.Scheme) == 0 {
				redurl.Scheme = link.Scheme
			}
		}
		if err == nil {
			return Action{
				Original: &link,
				Redir:    redurl,
				Status:   status,
			}
		} else {
			return Action{
				Original: &link,
				Err:      err,
				Status:   status,
			}
		}
	default:
		return Action{
			Original: &link,
			Redir:    nil,
			Status:   status,
		}
	}
}

func openValidFile(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	} else if info.IsDir() {
		return nil, errors.New("file is directory")
	}

	return os.Open(path)
}
