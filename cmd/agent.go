package cmd

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed skills/kit-kickoff/SKILL.md
var agentSkill []byte

func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Print or install the kit-kickoff agent skill",
		Long: "Print the bundled kit-kickoff SKILL.md, or install it for agent discovery.\n\n" +
			"The default install directory is ~/.fx/skills/kit-kickoff. For another client,\n" +
			"pass its complete skill directory with install --path. Existing different\n" +
			"content is never overwritten unless --force is given.",
		Args: cobra.NoArgs,
		// Skill discovery must not trigger setup prompts or update checks.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write(agentSkill)
			return err
		},
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "Install SKILL.md into a discoverable skill directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("path")
			if cmd.Flags().Changed("path") && dir == "" {
				return fmt.Errorf("--path must be a skill directory")
			}
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("find home directory: %w", err)
				}
				dir = filepath.Join(home, ".fx", "skills", "kit-kickoff")
			}
			force, _ := cmd.Flags().GetBool("force")
			path, err := installAgentSkill(dir, force)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Installed kit-kickoff skill:", path)
			return err
		},
	}
	install.Flags().String("path", "", "Explicit skill directory (SKILL.md is written inside)")
	install.Flags().Bool("force", false, "Replace an existing different SKILL.md")
	cmd.AddCommand(install)
	return cmd
}

func installAgentSkill(dir string, force bool) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create skill directory: %w", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if os.IsExist(err) {
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read existing skill: %w", readErr)
		}
		if bytes.Equal(existing, agentSkill) {
			return path, nil
		}
		if !force {
			return "", fmt.Errorf("%s already exists with different content; use --force to replace it", path)
		}
		if err := replaceAgentSkill(path); err != nil {
			return "", err
		}
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("create skill: %w", err)
	}
	_, writeErr := file.Write(agentSkill)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return "", fmt.Errorf("write skill: %w", writeErr)
		}
		return "", fmt.Errorf("close skill: %w", closeErr)
	}
	return path, nil
}

func replaceAgentSkill(path string) error {
	// Replace the directory entry instead of following an existing file symlink.
	file, err := os.CreateTemp(filepath.Dir(path), ".kit-kickoff-*")
	if err != nil {
		return fmt.Errorf("create replacement skill: %w", err)
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0644); err != nil {
		_ = file.Close()
		return err
	}
	_, writeErr := file.Write(agentSkill)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write replacement skill: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close replacement skill: %w", closeErr)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace skill: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newAgentCommand())
}
