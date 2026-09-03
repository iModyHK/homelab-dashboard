package registry

import (
	"errors"
	"testing"
)

func TestParseReference(t *testing.T) {
	cases := []struct {
		in       string
		registry string
		repo     string
		tag      string
		key      string
	}{
		{"nginx:alpine", "registry-1.docker.io", "library/nginx", "alpine", "nginx"},
		{"redis", "registry-1.docker.io", "library/redis", "latest", "redis"},
		{"malkurbi5/family-fund:0.1.9", "registry-1.docker.io", "malkurbi5/family-fund", "0.1.9", "malkurbi5/family-fund"},
		{"malkurbi5/radiology-portal", "registry-1.docker.io", "malkurbi5/radiology-portal", "latest", "malkurbi5/radiology-portal"},
		{"ghcr.io/immich-app/immich-server:v3.1.0", "ghcr.io", "immich-app/immich-server", "v3.1.0", "ghcr.io/immich-app/immich-server"},
		{"ghcr.io/immich-app/postgres:16-vectorchord0.3.0-pgvectors0.2.0", "ghcr.io", "immich-app/postgres", "16-vectorchord0.3.0-pgvectors0.2.0", "ghcr.io/immich-app/postgres"},
		{"localhost:5000/app:dev", "localhost:5000", "app", "dev", "localhost:5000/app"},
		{"docker.io/library/postgres:16-alpine", "registry-1.docker.io", "library/postgres", "16-alpine", "postgres"},
	}
	for _, tc := range cases {
		ref, err := ParseReference(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if ref.Registry != tc.registry || ref.Repository != tc.repo || ref.Tag != tc.tag {
			t.Fatalf("%s: got %+v", tc.in, ref)
		}
		if ref.RepoKey() != tc.key {
			t.Fatalf("%s: key %q", tc.in, ref.RepoKey())
		}
	}
}

func TestParseReferenceRejectsLocal(t *testing.T) {
	for _, in := range []string{"646a47c903c5", "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", "nginx@sha256:abc"} {
		if _, err := ParseReference(in); !errors.Is(err, ErrLocalImage) {
			t.Fatalf("%s: expected ErrLocalImage, got %v", in, err)
		}
	}
}

func TestParseChallenge(t *testing.T) {
	h := `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull,push"`
	p := parseChallenge(h)
	if p["realm"] != "https://auth.docker.io/token" || p["service"] != "registry.docker.io" || p["scope"] != "repository:library/nginx:pull,push" {
		t.Fatalf("%+v", p)
	}
}

func TestNormalizeRepoDigest(t *testing.T) {
	repo, digest := NormalizeRepoDigest("nginx@sha256:abc")
	if repo != "nginx" || digest != "sha256:abc" {
		t.Fatalf("%s %s", repo, digest)
	}
	repo, _ = NormalizeRepoDigest("ghcr.io/immich-app/immich-server@sha256:def")
	if repo != "ghcr.io/immich-app/immich-server" {
		t.Fatal(repo)
	}
}
