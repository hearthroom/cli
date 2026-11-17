package card

// LorebookSync keeps each named source's retry/delete identities independent.
type LorebookSync struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Version     int32             `json:"version,omitempty"`
	EntryIDs    []string          `json:"entryIds,omitempty"`
	EntryMap    map[string]string `json:"entryMap,omitempty"`
	SourceIDs   map[string]string `json:"sourceIds,omitempty"`
	EntryHashes map[string]string `json:"entryHashes,omitempty"`
	Upload      *LorebookUpload   `json:"upload,omitempty"`
	IgnoreLocal bool              `json:"ignoreLocal,omitempty"`
	Digest      string            `json:"digest,omitempty"`
}

func (s *State) LoadNamedBook(b LorebookSync) {
	s.LorebookID = b.ID
	s.LorebookName = b.Name
	s.LorebookVersion = b.Version
	s.LorebookEntryIDs = b.EntryIDs
	s.LorebookEntryMap = b.EntryMap
	s.LorebookSourceIDs = b.SourceIDs
	s.LorebookEntryHashes = b.EntryHashes
	s.LorebookUpload = b.Upload
	s.IgnoreLocalLorebook = b.IgnoreLocal
}
func (s State) NamedBookState(digest string) LorebookSync {
	return LorebookSync{ID: s.LorebookID, Name: s.LorebookName, Version: s.LorebookVersion, EntryIDs: s.LorebookEntryIDs, EntryMap: s.LorebookEntryMap, SourceIDs: s.LorebookSourceIDs, EntryHashes: s.LorebookEntryHashes, Upload: s.LorebookUpload, IgnoreLocal: s.IgnoreLocalLorebook, Digest: digest}
}
