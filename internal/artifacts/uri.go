package artifacts

import "strings"

const artifactURIPrefix = "artifact://"

func ArtifactURI(id string) string {
	return artifactURIPrefix + strings.TrimSpace(id)
}

func ParseArtifactURI(uri string) (id string, ok bool) {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, artifactURIPrefix) {
		return "", false
	}
	id = strings.TrimSpace(strings.TrimPrefix(uri, artifactURIPrefix))
	if id == "" {
		return "", false
	}
	return id, true
}
