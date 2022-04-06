package main

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestWriteMessage(t *testing.T) {
	GH_ORG_NAME := "flinkstech"
	n := new(slackNotifier)
	s := make(map[string]string)
	s["SHORT_SHA"] = "1234"
	s["BRANCH_NAME"] = "my-branch"
	s["_PR_NUMBER"] = ""
	s["REPO_NAME"] = "my-repo"
	s["COMMIT_SHA"] = "sha1234"

	b := &cbpb.Build{
		ProjectId:     "my-project-id",
		Id:            "some-build-id",
		Status:        cbpb.Build_SUCCESS,
		LogUrl:        "https://some.example.com/log/url?foo=bar",
		Substitutions: s,
	}

	got, err := n.writeMessage(b)
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	want := &slack.WebhookMessage{
		Attachments: []slack.Attachment{{
			Text: fmt.Sprintf("Build for commit '%s' on branch '%s' of repo `%s` has succeeded",
				s["SHORT_SHA"],
				s["BRANCH_NAME"],
				s["REPO_NAME"]),
			Color: "good",
			Actions: []slack.AttachmentAction{{
				Text: "View on GCB",
				Type: "button",
				URL:  "https://some.example.com/log/url?foo=bar&utm_campaign=google-cloud-build-notifiers&utm_medium=chat&utm_source=google-cloud-build",
			}, {
				Text: "View commit",
				Type: "button",
				URL: fmt.Sprintf("https://github.com/%s/%s/commit/%s",
					GH_ORG_NAME,
					s["REPO_NAME"],
					s["COMMIT_SHA"]),
			}},
		}},
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("writeMessage got unexpected diff: %s", diff)
	}
}
