package legalflow

import (
	"context"
	"errors"
	"fmt"
)

type Stage string

const (
	MatterIntake           Stage = "matter_intake"
	SignedDocumentDelivery Stage = "signed_document_delivery"
	DeadlineFollowUp       Stage = "deadline_follow_up"
)

type Submission struct {
	MatterID  string
	Stage     Stage
	Caption   string
	Filename  string
	MediaType string
	Image     []byte
}

type Decision struct {
	MatterID   string `json:"matter_id"`
	Stage      Stage  `json:"stage"`
	State      string `json:"state"`
	NextAction string `json:"next_action"`
	Asset      any    `json:"asset,omitempty"`
}

type Moderator interface {
	Flagged(context.Context, Submission) (bool, error)
}

type Transformer interface {
	Resize(context.Context, Submission) (any, error)
}

type Pipeline struct {
	moderator   Moderator
	transformer Transformer
}

func NewPipeline(m Moderator, t Transformer) *Pipeline {
	return &Pipeline{moderator: m, transformer: t}
}

func (p *Pipeline) Screen(ctx context.Context, in Submission) (Decision, error) {
	if in.MatterID == "" || in.Caption == "" || in.Filename == "" || in.MediaType == "" || len(in.Image) == 0 {
		return Decision{}, errors.New("matter_id, caption, filename, media type, and image are required")
	}
	state, action, ok := stageTransition(in.Stage)
	if !ok {
		return Decision{}, errors.New("stage must be matter_intake, signed_document_delivery, or deadline_follow_up")
	}
	flagged, err := p.moderator.Flagged(ctx, in)
	if err != nil {
		return Decision{}, fmt.Errorf("screen submission: %w", err)
	}
	if flagged {
		return Decision{MatterID: in.MatterID, Stage: in.Stage, State: "quarantined", NextAction: "counsel_review"}, nil
	}
	asset, err := p.transformer.Resize(ctx, in)
	if err != nil {
		return Decision{}, fmt.Errorf("prepare publication image: %w", err)
	}
	return Decision{MatterID: in.MatterID, Stage: in.Stage, State: state, NextAction: action, Asset: asset}, nil
}

func stageTransition(stage Stage) (string, string, bool) {
	switch stage {
	case MatterIntake:
		return "intake_ready", "open_matter", true
	case SignedDocumentDelivery:
		return "delivery_ready", "notify_recipient", true
	case DeadlineFollowUp:
		return "follow_up_ready", "schedule_reminder", true
	default:
		return "", "", false
	}
}
