package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	buildapi "github.com/openshift/origin/pkg/build/api"
	"github.com/openshift/origin/pkg/build/webhook"
)

// GitHubWebHook used for processing github webhook requests.
type GitHubWebHook struct{}

// New returns github webhook plugin.
func New() *GitHubWebHook {
	return &GitHubWebHook{}
}

type gitHubCommit struct {
	ID        string                     `json:"id,omitempty" yaml:"id,omitempty"`
	Author    buildapi.SourceControlUser `json:"author,omitempty" yaml:"author,omitempty"`
	Committer buildapi.SourceControlUser `json:"committer,omitempty" yaml:"committer,omitempty"`
	Message   string                     `json:"message,omitempty" yaml:"message,omitempty"`
}

type gitHubPushEvent struct {
	Ref        string       `json:"ref,omitempty" yaml:"ref,omitempty"`
	After      string       `json:"after,omitempty" yaml:"after,omitempty"`
	HeadCommit gitHubCommit `json:"head_commit,omitempty" yaml:"head_commit,omitempty"`
}

// Extract responsible for servicing webhooks from github.com.
func (p *GitHubWebHook) Extract(buildCfg *buildapi.BuildConfig, path string, req *http.Request) (build *buildapi.Build, proceed bool, err error) {
	if err = verifyRequest(req); err != nil {
		return
	}
	method := req.Header.Get("X-GitHub-Event")
	if method != "ping" && method != "push" {
		err = fmt.Errorf("Unknown X-GitHub-Event %s", method)
		return
	}
	if method == "ping" {
		proceed = false
		return
	}
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return
	}
	var event gitHubPushEvent
	if err = json.Unmarshal(body, &event); err != nil {
		return
	}
	proceed = webhook.GitRefMatches(event.Ref, buildCfg.Parameters.Source.Git.Ref)
	if !proceed {
		glog.V(2).Infof("Skipping build for '%s'.  Branch reference from '%s' does not match configuration", buildCfg, event)
	}

	build = &buildapi.Build{
		Parameters: buildapi.BuildParameters{
			Source: buildCfg.Parameters.Source,
			Revision: &buildapi.SourceRevision{
				Type: buildapi.BuildSourceGit,
				Git: &buildapi.GitSourceRevision{
					Commit:    event.HeadCommit.ID,
					Author:    event.HeadCommit.Author,
					Committer: event.HeadCommit.Committer,
					Message:   event.HeadCommit.Message,
				},
			},
			Strategy: buildCfg.Parameters.Strategy,
			Output:   buildCfg.Parameters.Output,
		},
	}

	return
}

func verifyRequest(req *http.Request) error {
	if method := req.Method; method != "POST" {
		return fmt.Errorf("Unsupported HTTP method %s", method)
	}
	if contentType := req.Header.Get("Content-Type"); contentType != "application/json" {
		return fmt.Errorf("Unsupported Content-Type %s", contentType)
	}
	if userAgent := req.Header.Get("User-Agent"); !strings.HasPrefix(userAgent, "GitHub-Hookshot/") {
		return fmt.Errorf("Unsupported User-Agent %s")
	}
	if req.Header.Get("X-GitHub-Event") == "" {
		return errors.New("Missing X-GitHub-Event")
	}
	return nil
}
