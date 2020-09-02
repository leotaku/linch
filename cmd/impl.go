package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/logrusorgru/aurora"
	http "github.com/valyala/fasthttp"
)

var regexpGuessOne = regexp.MustCompile(`(https?|file):\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,4}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)

type ActionKind int

const (
	ActionKindOk ActionKind = iota + 1
	ActionKindRedir
	ActionKindSemiRedir
	ActionKindErr
	ActionKindUrl
)

type Action struct {
	Kind     ActionKind
	Original *url.URL
	Redir    *url.URL
	Status   int
	Err      error
}

var au = aurora.NewAurora(true)

func (a Action) String() string {
	switch a.Kind {
	case ActionKindOk:
		return fmt.Sprintf("SUCCE %v: %v", au.Green(a.Status), a.Original.String())
	case ActionKindRedir:
		redir, _ := url.QueryUnescape(a.Redir.String())
		return fmt.Sprintf("REDIR %v: %v -> %v", au.Yellow(a.Status), a.Original.String(), redir)
	case ActionKindSemiRedir:
		redir, _ := url.QueryUnescape(a.Redir.String())
		return fmt.Sprintf("SEMIR %v: %v -> %v", au.Blue(a.Status), a.Original.String(), redir)
	case ActionKindErr:
		return fmt.Sprintf("ERROR %v: %v", au.Red(a.Status), a.Original.String())
	case ActionKindUrl:
		return fmt.Sprintf("URLER %v: %v", au.Magenta("XXX"), a.Err)
	default:
		panic("unreachable")
	}
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
	links := make(chan string, 100)
	rsps := make(chan Action, 100)
	go func() {
		wg := new(sync.WaitGroup)
		for link := range links {
			// Naive rate limiting
			time.Sleep(waitArg)
			wg.Add(1)
			go func(wg *sync.WaitGroup, link string) {
				handleRawLink(link, rsps)
				wg.Done()
			}(wg, link)
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
		rsps <- Action{Kind: ActionKindUrl, Err: err}
	} else {
		rsps <- handleLink(*url)
	}
}

func handleLink(link url.URL) Action {
	req := http.AcquireRequest()
	resp := http.AcquireResponse()
	defer http.ReleaseRequest(req)
	defer http.ReleaseResponse(resp)

	req.SetRequestURI(link.String())
	http.DoTimeout(req, resp, timeoutArg)
	status := resp.StatusCode()

	switch status {
	case 200:
		return Action{
			Kind:     ActionKindOk,
			Original: &link,
			Redir:    nil,
			Status:   status,
		}
	case 301, 308, 302, 307:
		redir := string(resp.Header.Peek("Location"))
		redurl, err := url.Parse(redir)
		if len(redir) != 0 && err == nil {
			kind := ActionKindRedir
			if status == 302 || status == 307 {
				kind = ActionKindSemiRedir
			}
			return Action{
				Kind:     kind,
				Original: &link,
				Redir:    redurl,
				Status:   status,
			}
		}
		fallthrough
	default:
		return Action{
			Kind:     ActionKindErr,
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
