package cmd

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var tripper = http.DefaultTransport

var regexpGuessOne = regexp.MustCompile(`(https?):\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,4}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)

type Action struct {
	Original Link
	Redir    string
	Status   int
	Error    error
}

type Link struct {
	Text string
	Path string
}

type Signal chan struct{}

func (s Signal) Send() {
	close(s)
}

func extractLinksForPaths(s *bufio.Scanner, links chan Link, stop Signal) {
	for s.Scan() {
		path, err := filepath.Abs(s.Text())
		if err != nil {
			panic("unreachable")
		}
		extractLinksForPath(path, links)
	}
	stop.Send()
}

func extractLinksForPath(path string, links chan Link) {
	file, err := openValidFile(path)
	if err != nil {
		return
	}
	defer file.Close()

	buf := bufio.NewScanner(file)
	for buf.Scan() {
		matches := regexpGuessOne.FindAllString(buf.Text(), -1)
		for _, rawurl := range matches {
			links <- Link{
				Text: rawurl,
				Path: path,
			}
		}
	}
}

func startLinkHandler() (chan Link, chan Action, Signal) {
	links := make(chan Link, 1000)
	rsps := make(chan Action, 100)
	stop := make(Signal)

	go func() {
		wg := new(sync.WaitGroup)
		for i := 0; i < limitArg; i++ {
			wg.Add(1)
			go func(wg *sync.WaitGroup) {
				for {
					select {
					case link := <-links:
						handleLink(link, links, rsps)
					default:
						select {
						case <-stop:
							wg.Done()
							return
						default:
						}
					}
				}
			}(wg)
		}
		wg.Wait()
		close(rsps)
	}()

	return links, rsps, stop
}

var hostmap sync.Map

func handleLink(link Link, links chan Link, rsps chan Action) {
	url, _ := url.Parse(link.Text)
	val, _ := hostmap.Load(url.Host)
	switch f := val.(type) {
	case time.Time:
		if time.Now().After(f) {
			hostmap.Delete(url.Host)
		} else {
			return
		}
	}

	req, _ := http.NewRequest("HEAD", link.Text, nil)
	resp, err := tripper.RoundTrip(req)

	switch {
	case err != nil:
		rsps <- Action{
			Original: link,
			Error:    err,
		}
	case resp.StatusCode == 429:
		after := time.Now()
		if it, ok := resp.Header["Retry-After"]; ok {
			if len(it) != 0 {
				secs, err := strconv.ParseUint(it[0], 10, 64)
				if err == nil {
					after.Add(time.Second * time.Duration(secs))
				} else {
					after.Add(time.Second)
				}
			} else {
				after.Add(time.Second)
			}
		} else {
			after.Add(time.Second)
		}
		hostmap.Store(resp.Request.URL.Host, after)
		links <- link
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		rsps <- handleRedirect(link, resp)
	default:
		rsps <- Action{
			Original: link,
			Status:   resp.StatusCode,
			Error:    err,
		}
	}
}

func handleRedirect(link Link, resp *http.Response) Action {
	redirs := resp.Header["Location"]
	if len(redirs) == 0 {
		return Action{
			Original: link,
			Error:    fmt.Errorf("missing location in redirection"),
			Status:   resp.StatusCode,
		}
	}

	redurl, err := url.Parse(redirs[0])
	if err != nil {
		return Action{
			Original: link,
			Status:   resp.StatusCode,
			Error:    fmt.Errorf("invalid location in redirection: '%s'", redirs[0]),
		}
	}
	if len(redurl.Host) == 0 {
		redurl.Host = resp.Request.URL.Host
	}
	if len(redurl.Scheme) == 0 {
		redurl.Scheme = resp.Request.URL.Scheme
	}

	return Action{
		Original: link,
		Redir:    redurl.String(),
		Status:   resp.StatusCode,
	}
}

func openValidFile(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	} else if info.IsDir() {
		return nil, fmt.Errorf("file is directory")
	}

	return os.Open(path)
}
