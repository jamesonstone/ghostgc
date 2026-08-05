package cacheartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Identity is the metadata-only identity of one filesystem entry.
type Identity struct {
	UID       uint32 `json:"uid"`
	Device    uint64 `json:"device"`
	Inode     uint64 `json:"inode"`
	Mode      uint32 `json:"mode"`
	Nlink     uint64 `json:"nlink"`
	Size      int64  `json:"size"`
	MTimeNs   int64  `json:"mtime_ns"`
	CTimeNs   int64  `json:"ctime_ns"`
	ATimeNs   int64  `json:"atime_ns"`
	BirthNs   int64  `json:"birth_ns,omitempty"`
	EntryType string `json:"entry_type"`
}

// SameObject reports whether both observations refer to the same filesystem object.
func (i Identity) SameObject(other Identity) bool {
	return i.UID == other.UID && i.Device == other.Device && i.Inode == other.Inode && i.EntryType == other.EntryType
}

// Equal reports whether every identity and manifest field is unchanged.
func (i Identity) Equal(other Identity) bool { return i == other }

// Digest returns the stable digest of this metadata identity.
func (i Identity) Digest() string {
	raw, _ := json.Marshal(i)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ArtifactID derives an opaque ID from provider and exact root/entry identity.
func ArtifactID(provider string, root, entry Identity) string {
	raw := fmt.Sprintf("%s\x00%d:%d\x00%d:%d", provider, root.Device, root.Inode, entry.Device, entry.Inode)
	sum := sha256.Sum256([]byte(raw))
	return "ca_" + hex.EncodeToString(sum[:16])
}

// ManifestDigest binds the relative path and complete metadata-only identity.
func ManifestDigest(relativePath string, identity Identity) string {
	raw, _ := json.Marshal(struct {
		RelativePath string   `json:"relative_path"`
		Identity     Identity `json:"identity"`
	}{relativePath, identity})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
