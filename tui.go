package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/oauth/api"
	"github.com/cli/oauth/device"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const (
	InitialState = iota
	RequestAcesss
	UploadingDiff
)

type model struct {
	diff         []byte
	state        int
	style        lipgloss.Style
	result       string
	code         *device.CodeResponse
	spinner      spinner.Model
	backgroundCh chan interface{}
	token        *api.AccessToken
}

func newModel() *model {
	diff, err := getDiff()
	if err != nil {
		log.Fatalf("error while retriving git, err: %s", err.Error())
	}
	token, err := getToken()
	if err != nil {
		log.Fatalf("error while retriving token: %s", err.Error())
	}
	var style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7F5283"))
	s := spinner.New()
	s.Spinner = spinner.Dot
	return &model{
		diff:         diff,
		style:        style,
		spinner:      s,
		backgroundCh: make(chan interface{}, 0),
		token:        token,
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case InitialState:
		// quit if we don't have any diff to upload.
		if len(m.diff) == 0 {
			m.result = m.style.Render("No diff found to share")
			return m, tea.Quit
		}
		// request for token if we don't have access to
		// gist.
		if m.token == nil {
			m.state = RequestAcesss
			cmd := m.requestAccess()
			return m, cmd
		}
		// upload the diff to gist in background routine,
		// so we can update the view as uploading gist.
		m.state = UploadingDiff
		var tick tea.Cmd
		m.spinner, tick = m.spinner.Update(spinner.Tick())
		go func() {
			gist, err := m.uploadDiff()
			if err != nil {
				err = fmt.Errorf("error while uploading diff to gist %s", err.Error())
			}
			m.backgroundCh <- UploadResponse{
				Err:          err,
				GistResponse: gist,
			}
		}()
		return m, tick
	case RequestAcesss:
		// check whether user given us access to gist
		// otherwise, update the spinner.
		select {
		case chRes := <-m.backgroundCh:
			res := chRes.(TokenResponse)
			if res.Err != nil {
				m.result = renderErrMsg(fmt.Sprintf("error while retriving token: %s", res.Err.Error()))
				return m, tea.Quit
			}
			// store the token to the config file, so it can
			// be reused for the next time.
			if err := storeToken(res.Token); err != nil {
				m.result = renderErrMsg(fmt.Sprintf("error while storing token: %s", err.Error()))
				return m, tea.Quit
			}
			// restart the state machine since we have access to
			// github gist to upload diff.
			m.token = res.Token
			m.state = InitialState
			return m, func() tea.Msg {
				return struct{}{}
			}
		default:
			// user haven't given us the access yet, so just update the
			// spinner.
			var tick tea.Cmd
			m.spinner, tick = m.spinner.Update(msg)
			return m, tick
		}
	case UploadingDiff:
		// quit the terminal if the diff has been uploaded, otherwise
		// update the spinner view.
		select {
		case chRes := <-m.backgroundCh:
			res := chRes.(UploadResponse)
			if res.Err != nil {
				m.result = renderErrMsg(res.Err.Error())
				return m, tea.Quit
			}
			m.result = m.style.Render(fmt.Sprintf(`
Diff has been uploaded sucessfully. Now you can use the following command to appy diff
wget -q -O - %s | git apply -v

Commands are copied to your clipboard so you can just paste it easily`, *res.GistResponse.Files["diffshare.diff"].RawURL))
			clipboard.WriteAll(fmt.Sprintf(`wget -q -O - %s | git apply -v`, *res.GistResponse.Files["diffshare.diff"].RawURL))
			return m, tea.Quit
		default:
			var tick tea.Cmd
			m.spinner, tick = m.spinner.Update(msg)
			return m, tick
		}
	}
	return m, nil
}

// upload diff will upload the diff to gihub gist.
func (m *model) uploadDiff() (*github.Gist, error) {
	// create github client using the user's device
	// token.
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: m.token.Token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// upload the diff to the github gist.
	gist, _, err := client.Gists.Create(context.TODO(), &github.Gist{
		Description: proto.String("created using diffshare"),
		Public:      proto.Bool(true),
		Files: map[github.GistFilename]github.GistFile{"diffshare.diff": github.GistFile{
			Content: proto.String(string(m.diff)),
		}},
	})
	return gist, err
}

// requestAccess will request user to grant us to upload
// diff to github gist.
func (m *model) requestAccess() tea.Cmd {
	code, err := device.RequestCode(http.DefaultClient, deviceURL, clientID, scope)
	if err != nil {
		m.result = renderErrMsg(fmt.Sprintf("error while generating github access code %s", err.Error()))
		return tea.Quit
	}
	m.code = code
	// poll the token in a serperate go routine so that we can update the view
	// as uploading gist.
	go func() {
		token, err := device.PollToken(http.DefaultClient, tokenURL, clientID, m.code)
		if err != nil {
			m.backgroundCh <- TokenResponse{Err: err}
		}
		err = storeToken(token)
		if err != nil {
			m.backgroundCh <- TokenResponse{Err: fmt.Errorf("error storing access token %s", err.Error())}
		}
		m.backgroundCh <- TokenResponse{Token: token}
	}()
	var tick tea.Cmd
	m.spinner, tick = m.spinner.Update(spinner.Tick())
	return tick
}

func (m *model) View() string {
	if m.state == RequestAcesss {
		return m.style.Render(fmt.Sprintf(`
%sWe request to grant us the gist access to store your git diff's
please open the given link: %s and enter the code %s`, m.spinner.View(), m.code.VerificationURI, m.code.UserCode))
	}
	return m.style.Render(fmt.Sprintf("%sUploading diff to gist", m.spinner.View()))
}

type TokenResponse struct {
	Token *api.AccessToken
	Err   error
}

type UploadResponse struct {
	GistResponse *github.Gist
	Err          error
}

func renderErrMsg(msg string) string {
	var style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#EB1D36"))
	return style.Render(msg)
}
