package cmd

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var tripper = http.DefaultTransport

var regexpGuessOne = regexp.MustCompile(`(https?):\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,4}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)

type Action struct {
	Original string
	Redir    string
	Status   int
	Error    error
}

type Link = string

type Signal chan struct{}

func (s Signal) Send() {
	close(s)
}

func extractLinksForPaths(s *bufio.Scanner, links chan Link, stop Signal) {
	for s.Scan() {
		extractLinksForPath(s.Text(), links)
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
			links <- rawurl
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

func handleLink(rawurl Link, links chan Link, rsps chan Action) {
	url, _ := url.Parse(rawurl)
	val, _ := hostmap.Load(url.Host)
	switch f := val.(type) {
	case time.Time:
		if time.Now().After(f) {
			hostmap.Delete(url.Host)
		} else {
			return
		}
	}

	req, _ := http.NewRequest("HEAD", rawurl, nil)
	resp, err := tripper.RoundTrip(req)

	switch {
	case err != nil:
		rsps <- Action{
			Original: rawurl,
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
		links <- rawurl
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		rsps <- handleRedirect(rawurl, resp)
	default:
		rsps <- Action{
			Original: rawurl,
			Redir:    "",
			Status:   resp.StatusCode,
			Error:    err,
		}
	}
}

func handleRedirect(rawurl string, resp *http.Response) Action {
	redirs := resp.Header["Location"]
	if len(redirs) == 0 {
		return Action{
			Original: rawurl,
			Error:    fmt.Errorf("missing location in redirection"),
			Status:   resp.StatusCode,
		}
	}

	redurl, err := url.Parse(redirs[0])
	if err != nil {
		return Action{
			Original: rawurl,
			Error:    fmt.Errorf("invalid location in redirection: '%s'", redirs[0]),
			Status:   resp.StatusCode,
		}
	}
	if len(redurl.Host) == 0 {
		redurl.Host = resp.Request.URL.Host
	}
	if len(redurl.Scheme) == 0 {
		redurl.Scheme = resp.Request.URL.Scheme
	}

	return Action{
		Original: rawurl,
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
