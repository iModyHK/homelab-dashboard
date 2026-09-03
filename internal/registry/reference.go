package registry

import (
	"errors"
	"regexp"
	"strings"
)

type Reference struct {
	Registry   string
	Repository string
	Tag        string
	Original   string
}

const dockerHubRegistry = "registry-1.docker.io"

var localIDRe = regexp.MustCompile(`^(sha256:)?[0-9a-f]{12,64}$`)

var ErrLocalImage = errors.New("image has no registry reference")

func ParseReference(image string) (Reference, error) {
	ref := Reference{Original: image, Tag: "latest"}
	if image == "" || localIDRe.MatchString(image) {
		return ref, ErrLocalImage
	}
	if at := strings.Index(image, "@"); at >= 0 {
		image = image[:at]
		ref.Tag = ""
	}
	name := image
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		if colon := strings.LastIndex(name[slash:], ":"); colon >= 0 {
			ref.Tag = name[slash+colon+1:]
			name = name[:slash+colon]
		}
	} else if colon := strings.LastIndex(name, ":"); colon >= 0 {
		ref.Tag = name[colon+1:]
		name = name[:colon]
	}
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 && looksLikeRegistry(parts[0]) {
		ref.Registry = parts[0]
		ref.Repository = parts[1]
	} else {
		ref.Registry = dockerHubRegistry
		ref.Repository = name
		if !strings.Contains(name, "/") {
			ref.Repository = "library/" + name
		}
	}
	if ref.Registry == "docker.io" || ref.Registry == "index.docker.io" {
		ref.Registry = dockerHubRegistry
		if !strings.Contains(ref.Repository, "/") {
			ref.Repository = "library/" + ref.Repository
		}
	}
	if ref.Tag == "" {
		return ref, ErrLocalImage
	}
	return ref, nil
}

func looksLikeRegistry(host string) bool {
	return strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost"
}

func (r Reference) ManifestURL() string {
	return "https://" + r.Registry + "/v2/" + r.Repository + "/manifests/" + r.Tag
}

func (r Reference) Scope() string {
	return "repository:" + r.Repository + ":pull"
}

func (r Reference) RepoKey() string {
	repo := r.Repository
	if r.Registry == dockerHubRegistry {
		repo = strings.TrimPrefix(repo, "library/")
		return repo
	}
	return r.Registry + "/" + repo
}

func NormalizeRepoDigest(repoDigest string) (string, string) {
	repo, digest, ok := strings.Cut(repoDigest, "@")
	if !ok {
		return "", ""
	}
	repo = strings.TrimPrefix(repo, "docker.io/")
	repo = strings.TrimPrefix(repo, "index.docker.io/")
	repo = strings.TrimPrefix(repo, "library/")
	return repo, digest
}
