// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

const (
	webhookURLSecretName = "webhookUrl"
	GH_ORG_NAME          = "flinkstech"
)

func main() {
	if err := notifiers.Main(new(slackNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type slackNotifier struct {
	filter notifiers.EventFilter

	webhookURL string
}

func (s *slackNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	s.filter = prd

	wuRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, webhookURLSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %w", cfg.Spec.Notification.Delivery, webhookURLSecretName, err)
	}
	wuResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, wuRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %w", wuRef, err)
	}
	wu, err := sg.GetSecret(ctx, wuResource)
	if err != nil {
		return fmt.Errorf("failed to get token secret: %w", err)
	}
	s.webhookURL = wu

	return nil
}

func (s *slackNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !s.filter.Apply(ctx, build) {
		return nil
	}

	log.Infof("sending Slack webhook for Build %q (status: %q)", build.Id, build.Status)
	msg, err := s.writeMessage(build)
	if err != nil {
		return fmt.Errorf("failed to write Slack message: %w", err)
	}

	return slack.PostWebhook(s.webhookURL, msg)
}

func (s *slackNotifier) writeMessage(build *cbpb.Build) (*slack.WebhookMessage, error) {
	BAD_STATUSES := [3]cbpb.Build_Status{cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT}
	var failedStep *cbpb.BuildStep
	statusMsg := ""

	var clr string
	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "good"
		statusMsg = "has succeeded"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		clr = "danger"
		statusMsg = "has failed"
	default:
		clr = "warning"
	}

	for _, step := range build.Steps {
		for _, status := range BAD_STATUSES {
			if step.Status == status {
				failedStep = step
				statusMsg += fmt.Sprintf(" %s", failedStep.Id)
			}
		}
	}

	txt := fmt.Sprintf(
		"Build for commit '%s' on branch '%s' %s",
		build.Substitutions["SHORT_SHA"],
		build.Substitutions["BRANCH_NAME"],
		statusMsg,
	)

	logURL, err := notifiers.AddUTMParams(build.LogUrl, notifiers.ChatMedium)
	if err != nil {
		return nil, fmt.Errorf("failed to add UTM params: %w", err)
	}

	var pr *slack.AttachmentAction
	if build.Substitutions["BRANCH_NAME"] != "master" && build.Substitutions["_PR_NUMBER"] != "" {
		*pr = slack.AttachmentAction{
			Text: "View PR",
			Type: "button",
			URL: fmt.Sprintf(
				"https://github.com/%s/%s/pull/%s",
				GH_ORG_NAME,
				build.Substitutions["REPO_NAME"],
				build.Substitutions["_PR_NUMBER"]),
		}
	}

	actions := []slack.AttachmentAction{{
		Text: "View on GCB",
		Type: "button",
		URL:  logURL,
	}, {
		Text: "View commit",
		Type: "button",
		URL: fmt.Sprintf(
			"https://github.com/%s/%s/commit/%s",
			GH_ORG_NAME,
			build.Substitutions["REPO_NAME"],
			build.Substitutions["COMMIT_SHA"]),
	}}

	if pr != nil {
		actions = append(actions, *pr)
	}

	atch := slack.Attachment{
		Text:    txt,
		Color:   clr,
		Actions: actions,
	}

	return &slack.WebhookMessage{Attachments: []slack.Attachment{atch}}, nil
}
