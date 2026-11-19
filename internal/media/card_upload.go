package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/hearthroom/cli/internal/api"
)

var errUploadIntentUnavailable = errors.New("provider does not offer library upload intents")

// A signed staging upload followed by an idempotent completion. A failure
// after the PUT retries completion with the same ticket.
func uploadViaTicket(ctx context.Context, c *api.Client, localPath, relativePath, roleID string) (UploadResult, error) {
	name := filepath.Base(localPath)
	if isQuickTimeName(name) {
		return UploadResult{}, errors.New(quickTimeMessage(name))
	}
	contentType := ContentType(name)
	if roleID == "" && contentType == "application/octet-stream" {
		// The intent requires a supported declared type. Multipart preserves
		// the server's byte sniffing when the file has no known extension.
		return UploadResult{}, errUploadIntentUnavailable
	}
	file, err := os.Open(localPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return UploadResult{}, err
	}
	var intent struct {
		Data struct {
			UploadID    string `json:"uploadId"`
			UploadURL   string `json:"uploadUrl"`
			ContentType string `json:"contentType"`
		} `json:"data"`
	}
	endpoint := "/image/"
	if roleID != "" {
		endpoint = "/role/" + url.PathEscape(roleID) + "/images/"
	}
	for attempt := 1; ; attempt++ {
		err = c.OpenPost(ctx, endpoint+"uploadIntent", map[string]any{"byteSize": stat.Size(), "contentType": contentType}, &intent)
		if err == nil {
			break
		}
		var response *api.Error
		if roleID == "" && errors.As(err, &response) && response.Code == "" &&
			(response.Status == http.StatusNotFound || response.Status == http.StatusMethodNotAllowed) {
			return UploadResult{}, errUploadIntentUnavailable
		}
		if isQuickTimeRefusal(err) {
			return UploadResult{}, errors.New(quickTimeMessage(name))
		}
		if attempt >= uploadAttempts || !retryable(ctx, err) {
			return UploadResult{}, err
		}
		if err := waitUploadRetry(ctx, attempt, err); err != nil {
			return UploadResult{}, err
		}
	}
	if intent.Data.UploadID == "" || intent.Data.UploadURL == "" {
		return UploadResult{}, errors.New("media storage returned no upload ticket")
	}
	destination, err := url.Parse(intent.Data.UploadURL)
	if err != nil || destination.User != nil || (destination.Scheme != "https" && destination.Scheme != "http") || destination.Host == "" {
		return UploadResult{}, errors.New("media storage returned an invalid destination")
	}
	complete := map[string]any{"uploadId": intent.Data.UploadID}
	if roleID == "" {
		complete["fileName"] = name
		if relativePath != "" {
			complete["relativePath"] = relativePath
		}
	}
	uploaded := false
	restaged := false
	for attempt := 1; ; attempt++ {
		var response uploadResponse
		if !uploaded {
			if _, err = file.Seek(0, io.SeekStart); err != nil {
				return UploadResult{}, err
			}
			request, e := http.NewRequestWithContext(ctx, http.MethodPut, intent.Data.UploadURL, io.NopCloser(file))
			if e != nil {
				return UploadResult{}, errors.New("cannot prepare media upload")
			}
			request.Header.Set("Content-Type", intent.Data.ContentType)
			request.ContentLength = stat.Size()
			// Do not forward provider bearer headers to the storage host.
			reply, e := c.HTTP.Do(request)
			if e != nil {
				if ue, ok := e.(*url.Error); ok {
					err = &url.Error{Op: ue.Op, URL: "media storage", Err: ue.Err}
				} else {
					err = e
				}
			} else {
				io.Copy(io.Discard, io.LimitReader(reply.Body, 4096))
				reply.Body.Close()
				if reply.StatusCode >= 200 && reply.StatusCode < 300 {
					uploaded = true
					err = nil
				} else {
					err = &api.Error{Status: reply.StatusCode, Method: http.MethodPut, URL: "media storage"}
				}
			}
		}
		if err == nil {
			err = c.OpenPost(ctx, endpoint+"uploadComplete", complete, &response)
		}
		if err == nil {
			if response.Data.ImageURL == "" {
				return UploadResult{}, fmt.Errorf("upload %s: no URL in response", name)
			}
			return response.Data, nil
		}
		if isQuickTimeRefusal(err) {
			return UploadResult{}, errors.New(quickTimeMessage(name))
		}
		var completeError *api.Error
		libraryComplete := roleID == "" && uploaded && errors.As(err, &completeError)
		if libraryComplete && !restaged && attempt < uploadAttempts &&
			completeError.Status == http.StatusNotFound && completeError.Code == "upload_missing" {
			// A completed storage move may precede a failed database commit.
			// Restore the same bytes and resume the same named upload once.
			restaged = true
			uploaded = false
			err = nil
			continue
		}
		if attempt >= uploadAttempts || (!retryable(ctx, err) &&
			!(libraryComplete && completeError.Status == http.StatusInternalServerError)) {
			return UploadResult{}, err
		}
		if err := waitUploadRetry(ctx, attempt, err); err != nil {
			return UploadResult{}, err
		}
		err = nil
	}
}

func waitUploadRetry(ctx context.Context, attempt int, err error) error {
	timer := time.NewTimer(retryDelay(attempt, err))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
