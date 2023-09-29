package cmd

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var urlmap sync.Map

var tripper = &http.Transport{
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
}

var regexpGuessOne = regexp.MustCompile(`(https?):\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,4}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)

type Action struct {
	Original Link
	Redir    string
	Status   int
	Error    error

	retryAfter time.Duration
}

type Link struct {
	URL  string
	Path string
}

func extractLinksForPaths(s *bufio.Scanner, links chan Link) {
	for s.Scan() {
		path, err := filepath.Abs(s.Text())
		if err != nil {
			panic("unreachable")
		}
		extractLinksForPath(path, links)
	}
	close(links)
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
				URL:  rawurl,
				Path: path,
			}
		}
	}
}

func startLinkHandler() (chan Link, chan Action) {
	inLinks := make(chan Link)
	rsps := make(chan Action, 100)

	wg := new(sync.WaitGroup)
	forwardLinks := make(chan Link, 1000)

	wg.Add(1)
	go func() {
		for link := range inLinks {
			wg.Add(1)
			forwardLinks <- link
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < limitArg; i++ {
			go func(wg *sync.WaitGroup) {
				for link := range forwardLinks {
					if val, ok := urlmap.Load(link.URL); !ok {
						action := handleLink(link)
						if action.Status == 429 {
							link := link
							go func() {
								// Forward
								randomize := time.Duration(1 + rand.Float32())
								time.Sleep(action.retryAfter + randomize)
								inLinks <- link
							}()
						} else {
							urlmap.Store(link.URL, action)
							// Return
							rsps <- action
							wg.Done()
						}
					} else if action, ok := val.(Action); ok {
						// Return
						rsps <- action
						wg.Done()
					}
				}
			}(wg)
		}

		wg.Wait()
		close(forwardLinks)
		close(rsps)
	}()

	return inLinks, rsps
}

func handleLink(link Link) Action {
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	req, err := http.NewRequestWithContext(ctx, "HEAD", link.URL, nil)
	defer cancel()
	if err != nil {
		return Action{
			Original: link,
			Error:    err,
		}
	}

	switch resp, err := tripper.RoundTrip(req); {
	case err != nil:
		return Action{
			Original: link,
			Error:    err,
		}
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return handleRedirect(link, resp)
	case resp.StatusCode == 429:
		return Action{
			Original:   link,
			Status:     resp.StatusCode,
			retryAfter: getRetryAfter(resp),
		}
	default:
		return Action{
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

func getRetryAfter(resp *http.Response) time.Duration {
	if it, ok := resp.Header["Retry-After"]; ok {
		if len(it) != 0 {
			secs, err := strconv.ParseUint(it[0], 10, 64)
			if err == nil {
				return time.Second * time.Duration(secs)
			}
		}
	}
	return time.Second * 15
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
