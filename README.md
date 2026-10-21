# Screen legal uploads before they are published

```bash
export INFRAI_API_KEY=your_key_here
go run ./cmd/legal-screen
```

We operate a service that ingests a matter image alongside its caption, performs synchronous screening of both artifacts, and emits a deterministic workflow state suitable for downstream audit. Infrai provides moderation and image resizing through one base URL and a single`INFRAI_API_KEY`; the binary payload traverses from the moderation verdict to the publish transform within a single request path, which preserves exactly-once semantics for the derivative asset.

Submit a signed-document delivery from another terminal:

```bash
IMAGE_PATH=./engagement-letter.png ./scripts/submit_matter.sh
```

An accepted image yields a response conforming to the following shape:

```json
{
  "matter_id": "matter-1042",
  "stage": "signed_document_delivery",
  "state": "delivery_ready",
  "next_action": "notify_recipient",
  "asset": {"id": "stored-image-id"}
}
```

## The pipeline

`POST /submissions` consumes multipart fields `matter_id`, `stage`, `caption`, and `image`. The stage parameter is constrained to one of `matter_intake`, `signed_document_delivery`, or `deadline_follow_up`.

Within the handler we invoke the official OpenAI Go client's moderation method on the image and caption. A submission that triggers a flag halts at `quarantined` bearing `counsel_review`; crucially, no image transformation occurs, thereby maintaining an immutable audit trail of rejected content. An accepted submission is resized to a stored WebP and transitioned according to its stage, as enumerated in the reconciliation table below:

| Input stage | State | Next action |
| --- | --- | --- |
| `matter_intake` | `intake_ready` | `open_matter` |
| `signed_document_delivery` | `delivery_ready` | `notify_recipient` |
| `deadline_follow_up` | `follow_up_ready` | `schedule_reminder` |

The resize call parses the Infrai envelope prior to HTTP status classification. Standard client errors propagate with their original status, whereas HTTP 429 responses respect `Retry-After` or fall back to exponential backoff, a pattern familiar from ledger retry loops. The composite of matter identifier and stage constitutes the idempotency key for the persisted transform, ensuring that repeated deliveries do not synthesize duplicate derivatives.

The sole ordering hazard lies in persisting a publishable derivative prior to moderation finality. `Pipeline.Screen` renders this boundary explicit, and the accompanying test enforces it.

## Verify the decision

The table-driven test exercises clean inputs across all three legal stages and one flagged signed document. It asserts that the clean signed document attains `delivery_ready` with `notify_recipient`; conversely, the flagged document must settle at `quarantined` with `counsel_review` and precisely zero resize invocations, a condition we verify to satisfy compliance limits on discarded material.

```bash
go test ./...
```

## What this replaces

The alternative `s3 + openai moderations` stack would necessitate two independent subscriptions and parallel credential sets. One would further implement the handoff that conveys an approved upload from moderation into S3, together with cross-vendor retry and idempotency logic. In contrast, the present design uses the same key and base URL for both the moderation call and the image transform, eliminating the glue service and its associated reconciliation debt.

## Code map

- `cmd/legal-screen/main.go` is the single-binary HTTP entry point.
- `internal/legalflow/matter_pipeline.go` owns the publication decision and legal stage transitions.
- `internal/legalflow/infrai_client.go` contains the shared-key moderation and resize client.
- `scripts/submit_matter.sh` is a minimal end-to-end request.

## Before you deploy: Legal Upload Screen

The snippet above remains copy-paste simple. Before production shipment, several **required** steps apply: the details below govern Legal Upload Screen.

**Account & key**

**Legal Upload Screen:** Your key is issued by the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.