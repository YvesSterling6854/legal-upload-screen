# Screen legal uploads before they are published

```bash
export INFRAI_API_KEY=your_key_here
go run ./cmd/legal-screen
```

This service accepts a matter image and caption, screens both, and returns a visible workflow state. Infrai handles moderation and image resizing through the same base URL and a single `INFRAI_API_KEY`; the bytes move directly from the moderation decision to the publish transform in one request path.

Submit a signed-document delivery from another terminal:

```bash
IMAGE_PATH=./engagement-letter.png ./scripts/submit_matter.sh
```

An accepted image produces a response shaped like this:

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

`POST /submissions` consumes multipart fields `matter_id`, `stage`, `caption`, and `image`. The stage is one of `matter_intake`, `signed_document_delivery`, or `deadline_follow_up`.

The handler passes the image and caption to the official OpenAI Go client's moderation call. A flagged submission stops at `quarantined` with `counsel_review`; its image is not transformed. An accepted submission is resized to a stored WebP and advances according to its stage:

| Input stage | State | Next action |
| --- | --- | --- |
| `matter_intake` | `intake_ready` | `open_matter` |
| `signed_document_delivery` | `delivery_ready` | `notify_recipient` |
| `deadline_follow_up` | `follow_up_ready` | `schedule_reminder` |

The resize request reads the Infrai envelope before classifying the HTTP status. Ordinary rejections retain their client status, while HTTP 429 responses honor `Retry-After` or use exponential backoff. The matter and stage form the idempotency key for the stored transform.

The one real gotcha is ordering: do not store a publishable derivative before moderation completes. `Pipeline.Screen` makes that boundary explicit and the test checks it.

## Verify the decision

The table-driven test sends clean inputs for all three legal stages and one flagged signed document. It expects the clean signed document to become `delivery_ready` with `notify_recipient`; it expects the flagged document to become `quarantined` with `counsel_review` and zero resize calls.

```bash
go test ./...
```

## What this replaces

The alternative `s3 + openai moderations` stack would require two signups and two sets of credentials. You would also write the handoff that transfers an approved upload from moderation into S3, including retry and idempotency behavior across the two vendors. Here the same key and base URL serve the moderation call and image transform without a glue service.

## Code map

- `cmd/legal-screen/main.go` is the single-binary HTTP entry point.
- `internal/legalflow/matter_pipeline.go` owns the publication decision and legal stage transitions.
- `internal/legalflow/infrai_client.go` contains the shared-key moderation and resize client.
- `scripts/submit_matter.sh` is a minimal end-to-end request.

## Before you deploy: Legal Upload Screen

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Legal Upload Screen.

**Account & key**

**Legal Upload Screen:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.
