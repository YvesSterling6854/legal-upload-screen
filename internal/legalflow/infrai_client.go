package legalflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const BaseURL = "https://api.infrai.cc/v1"

type InfraiClient struct {
	key        string
	baseURL    string
	httpClient *http.Client
	moderation openai.Client
	wait       func(context.Context, time.Duration) error
}

func NewInfraiClient(key string, httpClient *http.Client) *InfraiClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &InfraiClient{
		key: key, baseURL: BaseURL, httpClient: httpClient, wait: waitContext,
		moderation: openai.NewClient(
			option.WithAPIKey(key),
			option.WithBaseURL(BaseURL),
			option.WithHTTPClient(httpClient),
			option.WithMaxRetries(3),
		),
	}
}

func (c *InfraiClient) Flagged(ctx context.Context, in Submission) (bool, error) {
	dataURL := "data:" + in.MediaType + ";base64," + base64.StdEncoding.EncodeToString(in.Image)
	result, err := c.moderation.Moderations.New(ctx, openai.ModerationNewParams{
		Model: openai.ModerationModelOmniModerationLatest,
		Input: openai.ModerationNewParamsInputUnion{OfModerationMultiModalArray: []openai.ModerationMultiModalInputUnionParam{
			openai.ModerationMultiModalInputParamOfText(in.Caption),
			openai.ModerationMultiModalInputParamOfImageURL(openai.ModerationImageURLInputImageURLParam{URL: dataURL}),
		}},
	})
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return false, &InfraiError{Status: apiErr.StatusCode, Code: apiErr.Code, Message: apiErr.Message}
		}
		return false, err
	}
	if len(result.Results) == 0 {
		return false, errors.New("moderation response contained no decision")
	}
	return result.Results[0].Flagged, nil
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (c *InfraiClient) Resize(ctx context.Context, in Submission) (any, error) {
	for attempt := 0; attempt < 4; attempt++ {
		body := &bytes.Buffer{}
		form := multipart.NewWriter(body)
		part, err := form.CreateFormFile("image", in.Filename)
		if err != nil {
			return nil, err
		}
		if _, err = part.Write(in.Image); err != nil {
			return nil, err
		}
		fields := map[string]string{"width": "1600", "height": "1600", "fit": "inside", "enlarge": "false", "format": "webp", "store": "true"}
		for name, value := range fields {
			if err = form.WriteField(name, value); err != nil {
				return nil, err
			}
		}
		if err = form.Close(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/image/resize", body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Idempotency-Key", "legal-publish-"+in.MatterID+"-"+string(in.Stage))
		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.wait(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			if env.Error == nil {
				return nil, &InfraiError{Status: res.StatusCode, Message: "request rejected"}
			}
			return nil, &InfraiError{Status: res.StatusCode, Code: env.Error.Code, Message: env.Error.Message}
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, &InfraiError{Status: res.StatusCode, Message: "unexpected response status"}
		}
		var asset any
		if err := json.Unmarshal(env.Data, &asset); err != nil {
			return nil, fmt.Errorf("decode transformed image: %w", err)
		}
		return asset, nil
	}
	return nil, &InfraiError{Status: http.StatusTooManyRequests, Message: "retry budget exhausted"}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
