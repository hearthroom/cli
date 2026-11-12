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

// A signed staging upload followed by an idempotent completion. Card images
// never publish a named library path or fall back to the personal library.
func uploadCard(ctx context.Context, c *api.Client, localPath, roleID string) (UploadResult, error) {
	name := filepath.Base(localPath)
	if isQuickTimeName(name) {
		return UploadResult{}, errors.New(quickTimeMessage(name))
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
	endpoint := "/role/" + url.PathEscape(roleID) + "/images/"
	if err = c.OpenPost(ctx, endpoint+"uploadIntent", map[string]any{"byteSize": stat.Size(), "contentType": ContentType(name)}, &intent); err != nil {
		return UploadResult{}, err
	}
	if intent.Data.UploadID == "" || intent.Data.UploadURL == "" {
		return UploadResult{}, errors.New("card image storage returned no upload ticket")
	}
	destination, err := url.Parse(intent.Data.UploadURL)
	if err != nil || destination.User != nil || (destination.Scheme != "https" && destination.Scheme != "http") || destination.Host == "" {
		return UploadResult{}, errors.New("card image storage returned an invalid destination")
	}
	uploaded := false
	for attempt := 1; ; attempt++ {
		var response uploadResponse
		if !uploaded {
			if _, err = file.Seek(0, io.SeekStart); err != nil {
				return UploadResult{}, err
			}
			request, e := http.NewRequestWithContext(ctx, http.MethodPut, intent.Data.UploadURL, io.NopCloser(file))
			if e != nil {
				return UploadResult{}, errors.New("cannot prepare card image upload")
			}
			request.Header.Set("Content-Type", intent.Data.ContentType)
			request.ContentLength = stat.Size()
			// Do not forward provider bearer headers to the storage host.
			reply, e := c.HTTP.Do(request)
			if e != nil {
				if ue, ok := e.(*url.Error); ok {
					err = &url.Error{Op: ue.Op, URL: "card image storage", Err: ue.Err}
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
					err = &api.Error{Status: reply.StatusCode, Method: http.MethodPut, URL: "card image storage"}
				}
			}
		}
		if err == nil {
			err = c.OpenPost(ctx, endpoint+"uploadComplete", map[string]any{"uploadId": intent.Data.UploadID}, &response)
		}
		if err == nil {
			if response.Data.ImageURL == "" {
				return UploadResult{}, fmt.Errorf("card image %s: no URL in response", name)
			}
			return response.Data, nil
		}
		if attempt >= uploadAttempts || !retryable(ctx, err) {
			return UploadResult{}, err
		}
		select {
		case <-ctx.Done():
			return UploadResult{}, ctx.Err()
		case <-time.After(retryDelay(attempt, err)):
		}
		err = nil
	}
}
