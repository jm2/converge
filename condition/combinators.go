package condition

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// All returns a Condition that is met when every sub-condition is met.
// Wait blocks until all sub-conditions are met (checked sequentially).
func All(conditions ...extensions.Condition) extensions.Condition {
	return &allCondition{conditions: conditions}
}

// Any returns a Condition that is met when at least one sub-condition is met.
func Any(conditions ...extensions.Condition) extensions.Condition {
	return &anyCondition{conditions: conditions}
}

// Not returns a Condition that inverts another: met when inner is not met.
func Not(inner extensions.Condition) extensions.Condition {
	return &notCondition{inner: inner}
}

// ResourceIDs extracts all resource dependency IDs from a Condition tree.
// Used by the DSL to convert condition.Resource entries into DAG edges.
func ResourceIDs(c extensions.Condition) []string {
	var ids []string
	collectResourceIDs(c, &ids)
	return ids
}

// StripResources returns a new Condition with all Resource conditions removed.
// If only Resource conditions remain, returns nil (no runtime condition needed).
func StripResources(c extensions.Condition) extensions.Condition {
	return stripResources(c)
}

// --- allCondition ---

type allCondition struct {
	conditions []extensions.Condition
}

func (c *allCondition) Met(ctx context.Context) (bool, error) {
	for _, sub := range c.conditions {
		met, err := sub.Met(ctx)
		if err != nil {
			return false, err
		}
		if !met {
			return false, nil
		}
	}
	return true, nil
}

func (c *allCondition) Wait(ctx context.Context) error {
	for _, sub := range c.conditions {
		if err := sub.Wait(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *allCondition) String() string {
	parts := make([]string, len(c.conditions))
	for i, sub := range c.conditions {
		parts[i] = sub.String()
	}
	return fmt.Sprintf("all(%s)", strings.Join(parts, ", "))
}

// --- anyCondition ---

type anyCondition struct {
	conditions []extensions.Condition
}

func (c *anyCondition) Met(ctx context.Context) (bool, error) {
	for _, sub := range c.conditions {
		met, err := sub.Met(ctx)
		if err != nil {
			return false, err
		}
		if met {
			return true, nil
		}
	}
	return false, nil
}

func (c *anyCondition) Wait(ctx context.Context) error {
	// Poll until any sub-condition is met.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		for _, sub := range c.conditions {
			if met, _ := sub.Met(ctx); met {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *anyCondition) String() string {
	parts := make([]string, len(c.conditions))
	for i, sub := range c.conditions {
		parts[i] = sub.String()
	}
	return fmt.Sprintf("any(%s)", strings.Join(parts, ", "))
}

// --- notCondition ---

type notCondition struct {
	inner extensions.Condition
}

func (c *notCondition) Met(ctx context.Context) (bool, error) {
	met, err := c.inner.Met(ctx)
	if err != nil {
		return false, err
	}
	return !met, nil
}

func (c *notCondition) Wait(ctx context.Context) error {
	// Poll until the inner condition is NOT met.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if met, _ := c.inner.Met(ctx); !met {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *notCondition) String() string {
	return fmt.Sprintf("not(%s)", c.inner.String())
}

// --- tree helpers ---

func collectResourceIDs(c extensions.Condition, ids *[]string) {
	switch v := c.(type) {
	case *resourceCondition:
		*ids = append(*ids, v.id)
	case *allCondition:
		for _, sub := range v.conditions {
			collectResourceIDs(sub, ids)
		}
	case *anyCondition:
		for _, sub := range v.conditions {
			collectResourceIDs(sub, ids)
		}
	case *notCondition:
		collectResourceIDs(v.inner, ids)
	}
}

func stripResources(c extensions.Condition) extensions.Condition {
	switch v := c.(type) {
	case *resourceCondition:
		return nil
	case *allCondition:
		var kept []extensions.Condition
		for _, sub := range v.conditions {
			if stripped := stripResources(sub); stripped != nil {
				kept = append(kept, stripped)
			}
		}
		if len(kept) == 0 {
			return nil
		}
		if len(kept) == 1 {
			return kept[0]
		}
		return &allCondition{conditions: kept}
	case *anyCondition:
		var kept []extensions.Condition
		for _, sub := range v.conditions {
			if stripped := stripResources(sub); stripped != nil {
				kept = append(kept, stripped)
			}
		}
		if len(kept) == 0 {
			return nil
		}
		if len(kept) == 1 {
			return kept[0]
		}
		return &anyCondition{conditions: kept}
	case *notCondition:
		if stripped := stripResources(v.inner); stripped != nil {
			return &notCondition{inner: stripped}
		}
		return nil
	default:
		return c
	}
}
