package pkg

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type snapManager struct{}

func (s *snapManager) Name() string { return "snap" }

func (s *snapManager) IsInstalled(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "snap", "list", name)
	err := cmd.Run()
	return err == nil, nil
}

func (s *snapManager) Install(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "snap", "install", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("snap install %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (s *snapManager) Remove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "snap", "remove", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("snap remove %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
