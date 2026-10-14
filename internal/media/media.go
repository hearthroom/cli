// Package media uploads local assets to the provider's media library and
// tracks what was uploaded so unchanged files are skipped.
package media

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// Capabilities is the subset of GET /open/v1/image/list the CLI relies on.
type Capabilities struct {
	RelativePaths bool
	Moves         bool // renaming or moving a folder or file moves its path and URL
	Overwrite     bool
	LibraryPrefix string
	MaxFileBytes  int64
	Formats       []string
}

type listResponse struct {
	Code int `json:"code"`
	Data struct {
		Capabilities struct {
			RelativePaths bool     `json:"relativePaths"`
			Moves         bool     `json:"moves"`
			Overwrite     bool     `json:"overwrite"`
			MaxFileBytes  int64    `json:"maxFileBytes"`
			Formats       []string `json:"formats"`
		} `json:"capabilities"`
		LibraryPrefix string `json:"libraryPrefix"`
	} `json:"data"`
}

// Probe reads the library capabilities with one small list request.
func Probe(ctx context.Context, c *api.Client) (Capabilities, error) {
	var resp listResponse
	q := url.Values{"pageSize": {"1"}}
	if err := c.OpenGet(ctx, "/image/list", q, &resp); err != nil {
		return Capabilities{}, fmt.Errorf("read media library capabilities: %w", err)
	}
	return Capabilities{
		RelativePaths: resp.Data.Capabilities.RelativePaths,
		Moves:         resp.Data.Capabilities.Moves,
		Overwrite:     resp.Data.Capabilities.Overwrite,
		LibraryPrefix: strings.TrimRight(resp.Data.LibraryPrefix, "/"),
		MaxFileBytes:  resp.Data.Capabilities.MaxFileBytes,
		Formats:       resp.Data.Capabilities.Formats,
	}, nil
}

// UploadResult is the provider's upload response.
type UploadResult struct {
	ImageID         any    `json:"imageId"` // number, or string when the provider deduplicated by content
	ImageURL        string `json:"imageUrl"`
	Kind            string `json:"kind"`
	MimeType        string `json:"mimeType"`
	ByteSize        int64  `json:"byteSize"`
	FileName        string `json:"fileName"`
	Replaced        bool   `json:"replaced"`
	ModerationState string `json:"moderationState"`
}

type uploadResponse struct {
	Code int          `json:"code"`
	Data UploadResult `json:"data"`
}

// Upload sends one file. relativePath is used when the provider supports it;
// roleID may be empty. A .mov file is refused before anything is sent. A
// rate limit, a gateway error or a dropped connection is retried a few times
// with growing pauses; a named upload replaces the same library path, so
// sending it again after a lost reply does no harm.
func Upload(ctx context.Context, c *api.Client, localPath, relativePath, roleID string) (UploadResult, error) {
	for attempt := 1; ; attempt++ {
		res, err := uploadOnce(ctx, c, localPath, relativePath, roleID)
		if err == nil || attempt >= uploadAttempts || !retryable(ctx, err) {
			return res, err
		}
		select {
		case <-ctx.Done():
			return UploadResult{}, ctx.Err()
		case <-time.After(retryDelay(attempt, err)):
		}
	}
}

// uploadAttempts and retryBase bound the retries of one upload: pauses of
// about 1, 2 and 4 seconds, or what the provider asks for in retry_after.
var (
	uploadAttempts = 4
	retryBase      = time.Second
)

func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	// Transport failures (reset, timeout, EOF) come back as *url.Error.
	var ue *url.Error
	return errors.As(err, &ue)
}

func retryDelay(attempt int, err error) time.Duration {
	var e *api.Error
	if errors.As(err, &e) && e.RetryAfter > 0 {
		return min(time.Duration(e.RetryAfter)*time.Second, time.Minute)
	}
	d := retryBase << (attempt - 1)
	// ±50% so parallel workers that failed together do not retry together.
	return d/2 + time.Duration(rand.Int64N(int64(d)+1))
}

func uploadOnce(ctx context.Context, c *api.Client, localPath, relativePath, roleID string) (UploadResult, error) {
	name := filepath.Base(localPath)
	if isQuickTimeName(name) {
		return UploadResult{}, errors.New(quickTimeMessage(name))
	}
	f, err := os.Open(localPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer f.Close()
	fields := map[string][]string{}
	if relativePath != "" {
		fields["relativePath"] = []string{relativePath}
	}
	if roleID != "" {
		fields["roleId"] = []string{roleID}
	}
	var resp uploadResponse
	if err := c.UploadFile(ctx, "/image/upload", fields, "file", name, ContentType(name), f, &resp); err != nil {
		if isQuickTimeRefusal(err) {
			return UploadResult{}, errors.New(quickTimeMessage(name))
		}
		return UploadResult{}, err
	}
	if resp.Data.ImageURL == "" {
		return UploadResult{}, fmt.Errorf("upload %s: no URL in response", name)
	}
	return resp.Data, nil
}

// Outcome describes one asset after Sync.
type Outcome struct {
	Path     string `json:"path"`
	URL      string `json:"url"`
	Uploaded bool   `json:"uploaded"`
	Replaced bool   `json:"replaced,omitempty"`
	Previous string `json:"previous,omitempty"` // library path it was served from before a folder change; still in the library
	Err      string `json:"error,omitempty"`
}

// Folder picks the library folder for the card's assets: media.folder when
// set, else the folder recorded in state, else the folder earlier uploads
// went to, else one derived from the card name (fresh=true: not yet ours).
func Folder(f *card.Folder) (folder string, explicit, fresh bool) {
	if name, ok := f.LibraryFolder(); ok {
		return name, true, false
	}
	if f.State.AssetFolder != "" {
		return f.State.AssetFolder, false, false
	}
	keys := make([]string, 0, len(f.State.Assets))
	for k := range f.State.Assets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fn := f.State.Assets[k].FileName
		if i := strings.Index(fn, "/"); i > 0 {
			return fn[:i], false, false
		}
	}
	name, _ := f.LibraryFolder()
	return name, false, true
}

// FolderOf returns the library folder a served URL points into, or "" when
// the URL is not a <libraryPrefix>/<folder>/<file> library path.
func FolderOf(libraryPrefix, rawURL string) string {
	rest, ok := strings.CutPrefix(rawURL, strings.TrimRight(libraryPrefix, "/")+"/")
	if !ok || libraryPrefix == "" {
		return ""
	}
	seg, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	folder, err := url.PathUnescape(seg)
	if err != nil {
		return ""
	}
	return folder
}

// followMoves updates the recorded URLs of files that were renamed or moved in
// the library since the last push (the URL is the path, so the old one no longer
// serves), and forgets files that were deleted, so they are uploaded again.
// When the files moved to another folder, that folder becomes the card's.
func followMoves(ctx context.Context, c *api.Client, f *card.Folder, libraryPrefix string) error {
	wanted := map[string]string{} // id -> relative asset path
	for rel, a := range f.State.Assets {
		if a.ID != "" {
			wanted[a.ID] = rel
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	served := map[string]string{}
	complete := false
	for page := 1; page <= 20 && len(served) < len(wanted); page++ {
		var out struct {
			Data struct {
				Total     int64 `json:"total"`
				ImageList []struct {
					ID  any    `json:"id"`
					URL string `json:"imageUrl"`
				} `json:"imageList"`
			} `json:"data"`
		}
		q := url.Values{"kind": {"all"}, "pageSize": {"100"}, "pageNum": {fmt.Sprint(page)}}
		if err := c.OpenGet(ctx, "/image/list", q, &out); err != nil {
			return fmt.Errorf("read media library: %w", err)
		}
		for _, it := range out.Data.ImageList {
			if id := fmt.Sprint(it.ID); wanted[id] != "" {
				served[id] = it.URL
			}
		}
		if int64(page*100) >= out.Data.Total || len(out.Data.ImageList) == 0 {
			complete = true
			break
		}
	}
	folders := map[string]bool{}
	for id, rel := range wanted {
		a := f.State.Assets[rel]
		u, ok := served[id]
		switch {
		case !ok && complete:
			delete(f.State.Assets, rel) // deleted in the library: upload again
		case ok && u != a.URL:
			a.URL = u
			f.State.Assets[rel] = a
			if dir := FolderOf(libraryPrefix, u); dir != "" {
				folders[dir] = true
			}
		}
	}
	if len(folders) != 1 {
		return nil
	}
	var moved string
	for dir := range folders {
		moved = dir
	}
	if moved == f.State.AssetFolder {
		return nil
	}
	if name, explicit := f.LibraryFolder(); explicit && name != moved {
		return fmt.Errorf("this card's files were moved to the media folder %q in the library; set media.folder in card.json to %q so the next upload goes there too", moved, moved)
	}
	f.State.AssetFolder = moved
	return nil
}

// foreignFiles lists files already under folder/ that this card did not upload.
func foreignFiles(ctx context.Context, c *api.Client, f *card.Folder, libraryPrefix, folder string) ([]string, error) {
	var out struct {
		Data struct {
			ImageList []struct {
				FileName string `json:"fileName"`
				URL      string `json:"imageUrl"`
			} `json:"imageList"`
		} `json:"data"`
	}
	q := url.Values{"q": {folder + "/"}, "kind": {"all"}, "pageSize": {"20"}}
	if err := c.OpenGet(ctx, "/image/list", q, &out); err != nil {
		return nil, fmt.Errorf("check media folder %q: %w", folder, err)
	}
	ours := map[string]bool{}
	for _, a := range f.State.Assets {
		ours[a.URL] = true
	}
	var foreign []string
	for _, it := range out.Data.ImageList {
		if strings.HasPrefix(it.URL, libraryPrefix+"/"+escapePath(folder)+"/") && !ours[it.URL] {
			foreign = append(foreign, it.FileName)
		}
	}
	return foreign, nil
}

// escapePath percent-escapes each segment the way the library serves it.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// AddDirURLs sets the URL of each referenced asset directory from the files
// already in urls: a directory is served at the URL its files share. Push and
// the commands that compare against the last push both need it, or a card that
// builds paths at runtime looks changed after every push.
func AddDirURLs(f *card.Folder, urls map[string]string) {
	for _, dir := range f.AssetDirs() {
		for rel, u := range urls {
			if tail := escapePath(strings.TrimPrefix(rel, dir)); strings.HasPrefix(rel, dir) && strings.HasSuffix(u, "/"+tail) {
				urls[dir] = strings.TrimSuffix(u, tail)
				break
			}
		}
	}
}

// DefaultConcurrency is how many files Sync uploads at once unless told
// otherwise; MaxConcurrency caps any setting.
const (
	DefaultConcurrency = 6
	MaxConcurrency     = api.MaxIdleConnsPerHost
)

// SyncOptions control Sync.
type SyncOptions struct {
	// DryRun reports what would be uploaded without sending anything.
	DryRun bool
	// Concurrency is the number of parallel uploads; 0 means DefaultConcurrency.
	Concurrency int
	// Save writes the folder state. Sync calls it every few seconds while
	// uploading and once when it stops, so an interrupted push resumes with
	// the files that are left.
	Save func() error
	// Progress is called after every finished upload, from one goroutine.
	Progress func(Progress)
}

// Progress counts the uploads of one Sync.
type Progress struct {
	Done   int // finished, failed ones included
	Failed int
	Total  int // files that need an upload
}

// saveEvery spaces the state checkpoints during an upload.
var saveEvery = 2 * time.Second

// Sync uploads every referenced asset of the folder that is new, changed or
// served from another library folder, records it in the folder state and
// returns the path→URL map, including referenced directories. assets/<path>
// goes to <folder>/<path> (see Folder). Failures are reported per file; the
// map still contains every asset that has a URL. Outcomes keep the order of
// the folder's references however the uploads finish. When ctx is cancelled
// Sync stops starting uploads, saves what finished and returns ctx's error.
func Sync(ctx context.Context, c *api.Client, f *card.Folder, caps Capabilities, opts SyncOptions) (map[string]string, []Outcome, error) {
	dryRun := opts.DryRun
	present, missing := f.AssetRefs()
	urls := map[string]string{}
	var outcomes []Outcome
	if caps.RelativePaths {
		if err := followMoves(ctx, c, f, caps.LibraryPrefix); err != nil {
			return nil, nil, err
		}
	}
	folder, _, fresh := Folder(f)
	if caps.RelativePaths && fresh && len(present) > 0 {
		foreign, err := foreignFiles(ctx, c, f, caps.LibraryPrefix, folder)
		if err != nil {
			return nil, nil, err
		}
		if len(foreign) > 0 {
			return nil, nil, fmt.Errorf("media folder %q already holds files this card did not upload (%s). Set media.folder in card.json to a new name, or to %q to share that folder on purpose; an upload with the same path replaces that file for every card that uses it", folder, strings.Join(foreign[:min(3, len(foreign))], ", "), folder)
		}
	}
	for _, m := range missing {
		outcomes = append(outcomes, Outcome{Path: m, Err: "file not found"})
	}
	if f.State.Assets == nil {
		f.State.Assets = map[string]card.Asset{}
	}
	// Decide every file first; only the uploads run in parallel.
	var jobs []uploadJob
	for _, rel := range present {
		local := filepath.Join(f.Dir, filepath.FromSlash(rel))
		digest, err := card.FileDigest(local)
		if err != nil {
			outcomes = append(outcomes, Outcome{Path: rel, Err: err.Error()})
			continue
		}
		remotePath := ""
		if caps.RelativePaths {
			remotePath = path.Join(folder, strings.TrimPrefix(rel, card.AssetsDir+"/"))
		}
		prev, had := f.State.Assets[rel]
		// Compare served URLs, not names: a library rename changes the shown
		// name but not the path a file is served from.
		want := ""
		if remotePath != "" && caps.LibraryPrefix != "" {
			want = caps.LibraryPrefix + "/" + escapePath(remotePath)
		}
		moved := had && want != "" && prev.FileName != "" && prev.URL != "" && prev.URL != want
		if had && prev.SHA256 == digest && prev.URL != "" && !moved {
			urls[rel] = prev.URL
			outcomes = append(outcomes, Outcome{Path: rel, URL: prev.URL})
			continue
		}
		previous := ""
		if moved {
			previous = prev.URL
		}
		if caps.MaxFileBytes > 0 {
			if st, err := os.Stat(local); err == nil && st.Size() > caps.MaxFileBytes {
				outcomes = append(outcomes, Outcome{Path: rel, Err: fmt.Sprintf("larger than the provider limit (%d bytes)", caps.MaxFileBytes)})
				continue
			}
		}
		if isQuickTimeName(local) {
			// Refused here too, so --dry-run reports it instead of "would upload".
			outcomes = append(outcomes, Outcome{Path: rel, Err: quickTimeMessage(filepath.Base(local))})
			continue
		}
		if dryRun {
			outcomes = append(outcomes, Outcome{Path: rel, Uploaded: true, Previous: previous})
			continue
		}
		// The outcome slot is filled when the upload finishes.
		jobs = append(jobs, uploadJob{slot: len(outcomes), rel: rel, local: local, remotePath: remotePath, digest: digest, previous: previous})
		outcomes = append(outcomes, Outcome{Path: rel})
	}
	if !dryRun && caps.RelativePaths {
		// Recorded before the uploads so every checkpoint names the folder.
		f.State.AssetFolder = folder
	}
	if len(jobs) > 0 {
		if err := runUploads(ctx, c, f, jobs, urls, outcomes, opts); err != nil {
			return nil, nil, err
		}
	}
	// A referenced directory is served at the URL its files share, so card
	// code can append a file name at runtime.
	if caps.RelativePaths {
		AddDirURLs(f, urls)
	} else {
		for _, dir := range f.AssetDirs() {
			outcomes = append(outcomes, Outcome{Path: dir, Err: "this provider cannot serve files by folder path; reference each file literally"})
		}
	}
	return urls, outcomes, nil
}

type uploadJob struct {
	slot                                     int
	rel, local, remotePath, digest, previous string
}

type uploadDone struct {
	job uploadJob
	res UploadResult
	err error
}

// giveUpAfter is how many uploads in a row may fail on a transient error
// (after their own retries) before the push stops: by then the provider or
// the network is down, and retrying every remaining file would take hours.
var giveUpAfter = 10

// runUploads sends the jobs with a bounded number of workers. Only this
// goroutine touches the folder state, urls and outcomes.
func runUploads(parent context.Context, c *api.Client, f *card.Folder, jobs []uploadJob, urls map[string]string, outcomes []Outcome, opts SyncOptions) error {
	ctx, stop := context.WithCancel(parent)
	defer stop()
	workers := opts.Concurrency
	if workers <= 0 {
		workers = DefaultConcurrency
	}
	workers = min(workers, MaxConcurrency, len(jobs))
	queue := make(chan uploadJob)
	results := make(chan uploadDone, len(jobs))
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				res, err := Upload(ctx, c, j.local, j.remotePath, "")
				results <- uploadDone{job: j, res: res, err: err}
			}
		}()
	}
	go func() {
		defer close(queue)
		for _, j := range jobs {
			select {
			case queue <- j:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	save := func() {
		if opts.Save != nil {
			_ = opts.Save()
		}
	}
	p := Progress{Total: len(jobs)}
	lastSave := time.Now()
	if opts.Progress != nil {
		opts.Progress(p)
	}
	var streak int
	var gaveUp error
	for d := range results {
		if d.err != nil && ctx.Err() != nil {
			continue // stopped by the cancellation, not a failure of this file
		}
		p.Done++
		if d.err != nil {
			p.Failed++
			outcomes[d.job.slot] = Outcome{Path: d.job.rel, Err: d.err.Error()}
			if retryable(context.Background(), d.err) {
				streak++
			}
			if streak >= giveUpAfter && gaveUp == nil {
				gaveUp = fmt.Errorf("the provider is not answering: %d uploads in a row failed (last: %w)", streak, d.err)
				stop()
			}
		} else {
			streak = 0
			// For named uploads the provider returns the stable library URL
			// (<libraryPrefix>/<relativePath>) as imageUrl; for unnamed ones it
			// returns the object URL. Either is what the card should reference.
			served := d.res.ImageURL
			id := ""
			if d.res.ImageID != nil {
				id = fmt.Sprint(d.res.ImageID)
			}
			f.State.Assets[d.job.rel] = card.Asset{SHA256: d.job.digest, URL: served, FileName: d.res.FileName, ID: id}
			urls[d.job.rel] = served
			outcomes[d.job.slot] = Outcome{Path: d.job.rel, URL: served, Uploaded: true, Replaced: d.res.Replaced, Previous: d.job.previous}
		}
		if opts.Progress != nil {
			opts.Progress(p)
		}
		if time.Since(lastSave) >= saveEvery {
			save()
			lastSave = time.Now()
		}
	}
	if err := parent.Err(); err != nil {
		gaveUp = err
	}
	if gaveUp != nil {
		save()
		return fmt.Errorf("upload stopped after %d of %d files; the finished ones are saved, push again to send the rest: %w", p.Done-p.Failed, p.Total, gaveUp)
	}
	return nil
}
