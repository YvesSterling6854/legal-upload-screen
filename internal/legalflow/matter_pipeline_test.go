package legalflow

import (
	"context"
	"testing"
)

type fixedModerator bool

func (m fixedModerator) Flagged(context.Context, Submission) (bool, error) { return bool(m), nil }

type countingTransformer struct{ calls int }

func (t *countingTransformer) Resize(context.Context, Submission) (any, error) {
	t.calls++
	return map[string]string{"id": "image-ready"}, nil
}

func TestScreenTransitions(t *testing.T) {
	tests := []struct {
		name       string
		stage      Stage
		flagged    bool
		state      string
		nextAction string
		transforms int
	}{
		{"clean intake opens matter", MatterIntake, false, "intake_ready", "open_matter", 1},
		{"clean signature notifies recipient", SignedDocumentDelivery, false, "delivery_ready", "notify_recipient", 1},
		{"clean deadline schedules reminder", DeadlineFollowUp, false, "follow_up_ready", "schedule_reminder", 1},
		{"flagged signature waits for counsel", SignedDocumentDelivery, true, "quarantined", "counsel_review", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transformer := &countingTransformer{}
			pipeline := NewPipeline(fixedModerator(tc.flagged), transformer)
			got, err := pipeline.Screen(context.Background(), Submission{
				MatterID: "matter-1042", Stage: tc.stage, Caption: "Signed engagement letter",
				Filename: "engagement.png", MediaType: "image/png", Image: []byte("image"),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.state || got.NextAction != tc.nextAction {
				t.Fatalf("got state=%q action=%q", got.State, got.NextAction)
			}
			if transformer.calls != tc.transforms {
				t.Fatalf("transform calls=%d, want %d", transformer.calls, tc.transforms)
			}
		})
	}
}
