package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

func RunCheck(ctx context.Context, checker *Checker, dockerClient *docker.Client, st *store.Store, images []string) error {
	if len(images) == 0 {
		return st.PruneImageUpdates(ctx, nil)
	}
	sort.Strings(images)
	local, err := dockerClient.ListImages(ctx)
	if err != nil {
		return fmt.Errorf("list local images: %w", err)
	}
	results := checker.Check(ctx, images, local)
	now := time.Now().Unix()
	records := make([]store.ImageUpdateRecord, 0, len(results))
	var failures int
	for _, r := range results {
		if r.Error != "" && r.Error != "local build" && r.Error != "no local digest" {
			failures++
		}
		records = append(records, store.ImageUpdateRecord{
			Image: r.Image, LocalDigest: r.LocalDigest, RemoteDigest: r.RemoteDigest,
			UpdateAvailable: r.UpdateAvailable, CheckedAt: now, Error: r.Error,
		})
	}
	if err := st.UpsertImageUpdates(ctx, records); err != nil {
		return err
	}
	if err := st.PruneImageUpdates(ctx, images); err != nil {
		return err
	}
	if failures == len(results) && len(results) > 0 {
		return errors.New("every registry lookup failed")
	}
	return nil
}
