//go:build darwin

package firewall

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/version"
)

const (
	pfConf    = "/etc/pf.conf"
	pfAnchor  = version.Name
	anchorDir = "/etc/pf.anchors"
)

var pfAction = map[string]string{"block": "block", "allow": "pass"}
var pfDirection = map[string]string{"outbound": "out", "inbound": "in"}

// Check determines whether the pf rule exists in the converge anchor file with
// the desired attributes. Matching the comment tag alone is insufficient: a
// rule flipped block->allow or re-pointed to another port/source keeps its tag,
// so Check compares the rule body against the freshly rendered pfRule(). Any
// mismatch is reported as drift so Apply (filter-by-tag then re-add) corrects it.
func (f *Firewall) Check(_ context.Context) (*extensions.State, error) {
	if err := f.validErr(); err != nil {
		return nil, err
	}
	exists, match, err := f.ruleState()
	if err != nil {
		return nil, fmt.Errorf("check firewall rule %q: %w", f.Name, err)
	}

	if f.State == "absent" {
		return checkResult(f.Name, exists, false)
	}

	if exists && match {
		return &extensions.State{InSync: true}, nil
	}
	action := "add"
	if exists {
		action = "modify"
	}
	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{{
			Property: "rule",
			From:     boolToState(exists),
			To:       "present",
			Action:   action,
		}},
	}, nil
}

// Apply creates or removes the pf rule, then reloads pf.
func (f *Firewall) Apply(_ context.Context) (*extensions.Result, error) {
	if err := f.validErr(); err != nil {
		return nil, err
	}
	if f.State == "absent" {
		return f.removeRule()
	}
	return f.addRule()
}

func (f *Firewall) addRule() (*extensions.Result, error) {
	if err := f.ensureAnchor(); err != nil {
		return nil, err
	}

	rules, err := readAnchorFile()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read anchor: %w", err)
	}

	tag := f.commentTag()
	filtered := filterRules(rules, tag)
	filtered = append(filtered, f.pfRule()+" "+tag)

	if err := writeAnchorFile(filtered); err != nil {
		return nil, err
	}
	if err := pfctlReload(); err != nil {
		return nil, err
	}

	return resultChanged("added")
}

func (f *Firewall) removeRule() (*extensions.Result, error) {
	rules, err := readAnchorFile()
	if err != nil {
		if os.IsNotExist(err) {
			return &extensions.Result{Changed: false, Status: extensions.StatusOK, Message: "already absent"}, nil
		}
		return nil, fmt.Errorf("read anchor: %w", err)
	}

	tag := f.commentTag()
	filtered := filterRules(rules, tag)

	if len(filtered) == len(rules) {
		return &extensions.Result{Changed: false, Status: extensions.StatusOK, Message: "already absent"}, nil
	}

	if err := writeAnchorFile(filtered); err != nil {
		return nil, err
	}
	if err := pfctlReload(); err != nil {
		return nil, err
	}

	return resultChanged("removed")
}

// ruleState reports whether a tagged rule for this resource exists in the anchor
// file and, if so, whether its body matches the desired rule. A rule "exists" if
// any line carries the comment tag; it "matches" only if the rule text before the
// tag equals the freshly rendered pfRule().
func (f *Firewall) ruleState() (exists, match bool, err error) {
	rules, err := readAnchorFile()
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}

	tag := f.commentTag()
	want := f.pfRule()
	count := 0
	for _, line := range rules {
		if !strings.Contains(line, tag) {
			continue
		}
		exists = true
		count++
		body := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), tag))
		match = body == want
	}
	// In sync only with exactly one tagged rule that matches; duplicates (e.g.
	// out-of-band edits) are drift so Apply (filter-all-by-tag then re-add one)
	// collapses them to the canonical rule.
	if count > 1 {
		match = false
	}
	return exists, match, nil
}

// filterRules returns lines that do not contain the given tag.
func filterRules(rules []string, tag string) []string {
	var out []string
	for _, line := range rules {
		if !strings.Contains(line, tag) {
			out = append(out, line)
		}
	}
	return out
}

func (f *Firewall) commentTag() string {
	return fmt.Sprintf("# converge:%s", f.Name)
}

// pfRule generates the pf rule string. Port always means destination port.
func (f *Firewall) pfRule() string {
	var parts []string
	parts = append(parts, pfAction[f.Action], pfDirection[f.Direction], "proto", f.Protocol)

	if f.Source != "" {
		parts = append(parts, "from", f.Source)
	} else {
		parts = append(parts, "from", "any")
	}

	if f.Dest != "" {
		parts = append(parts, "to", f.Dest, "port", fmt.Sprintf("%d", f.Port))
	} else {
		parts = append(parts, "to", "any", "port", fmt.Sprintf("%d", f.Port))
	}

	return strings.Join(parts, " ")
}

// containsExactLine checks if content has a line matching s exactly
// (after trimming whitespace), preventing substring false positives.
func containsExactLine(content, s string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == s {
			return true
		}
	}
	return false
}

func anchorFilePath() string {
	return filepath.Join(anchorDir, pfAnchor)
}

func readAnchorFile() ([]string, error) {
	data, err := os.ReadFile(anchorFilePath())
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func writeAnchorFile(rules []string) error {
	if err := os.MkdirAll(anchorDir, 0755); err != nil {
		return fmt.Errorf("create anchor dir: %w", err)
	}
	content := strings.Join(rules, "\n")
	if content != "" {
		content += "\n"
	}
	return atomicWriteFile(anchorFilePath(), []byte(content), 0600, nil)
}

// atomicWriteFile writes data to a temp file, optionally validates it,
// then atomically renames to the target path. Shared by writeAnchorFile
// and ensureAnchor.
func atomicWriteFile(target string, data []byte, perm os.FileMode, validate func(tmp string) error) error {
	tmp := target + ".converge.tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write temp %s: %w", target, err)
	}
	if validate != nil {
		if err := validate(tmp); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", target, err)
	}
	return nil
}

// ensureAnchor adds the converge anchor to pf.conf if missing.
// Validates the new config with pfctl -nf before committing.
func (f *Firewall) ensureAnchor() error {
	data, err := os.ReadFile(pfConf)
	if err != nil {
		return fmt.Errorf("read %s: %w", pfConf, err)
	}

	anchorLine := fmt.Sprintf("anchor \"%s\"", pfAnchor)
	loadLine := fmt.Sprintf("load anchor \"%s\" from \"%s\"", pfAnchor, anchorFilePath())

	content := string(data)
	hasAnchor := containsExactLine(content, anchorLine)
	hasLoad := containsExactLine(content, loadLine)
	if hasAnchor && hasLoad {
		return nil
	}

	if !hasAnchor {
		content = strings.TrimRight(content, "\n") + "\n" + anchorLine + "\n"
	}
	if !hasLoad {
		content = strings.TrimRight(content, "\n") + "\n" + loadLine + "\n"
	}

	return atomicWriteFile(pfConf, []byte(content), 0600, func(tmp string) error {
		// macOS pf has no stable userspace API: pfctl is the standard mechanism.
		if out, err := exec.Command("/sbin/pfctl", "-nf", tmp).CombinedOutput(); err != nil {
			return fmt.Errorf("pf.conf validation failed: %s: %w", out, err)
		}
		return nil
	})
}
