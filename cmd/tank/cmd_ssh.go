package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newSSHCmd(projectPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "ssh [name] [-- ssh_args...]",
		Short: "SSH into a running VM (auto-starts if needed)",
		Long:  "Connect to a VM instance via SSH, building and starting it if necessary. Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Split args at -- into instance name args and SSH args
			var nameArgs, extraSSHArgs []string
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx >= 0 {
				nameArgs = args[:dashIdx]
				extraSSHArgs = args[dashIdx:]
			} else {
				nameArgs = args
			}

			// Determine instance name
			instanceName, err := resolveInstanceName(*projectPath, nameArgs)
			if err != nil {
				return err
			}

			// Ensure instance is running (build/create/start as needed)
			if err := ensureRunning(*projectPath, instanceName, 2, 4096, false); err != nil {
				return err
			}

			inst, err := instance.Load(instanceName)
			if err != nil {
				return fmt.Errorf("loading instance: %w", err)
			}

			// Get current username (matches cloud-init user)
			currentUser, err := user.Current()
			if err != nil {
				return fmt.Errorf("getting current user: %w", err)
			}

			// Get VM IP address with retries
			var ip string
			ipAttempt := 0
			err = ui.RunWithWaiting(os.Stderr, "Waiting for IP address", 2*time.Second, func() (bool, error) {
				ipAttempt++
				addr, err := inst.IPAddress()
				if err != nil {
					return false, err
				}
				if addr != "" {
					ip = addr
					return true, nil
				}
				if ipAttempt >= 30 {
					diagnosis := diagnoseIPTimeout(inst)
					return false, fmt.Errorf("timed out waiting for VM IP address.%s", diagnosis)
				}
				return false, nil
			})
			if err != nil {
				return err
			}

			// Build SSH command
			sshArgs := []string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
			}

			if shouldDisableTTY(extraSSHArgs) {
				sshArgs = append(sshArgs, "-T")
			}

			sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", currentUser.Username, ip))

			// Append any extra args passed after --
			sshArgs = append(sshArgs, extraSSHArgs...)

			// Wait for SSH to become available by probing quietly
			sshAttempt := 0
			err = ui.RunWithWaiting(os.Stderr, "Waiting for SSH", 2*time.Second, func() (bool, error) {
				sshAttempt++
				probe := exec.Command("ssh", append(sshArgs, "true")...)
				if output, err := probe.CombinedOutput(); err != nil {
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) && exitErr.ExitCode() == 255 {
						if sshAttempt >= 30 {
							return false, fmt.Errorf("timed out waiting for SSH connection")
						}
						return false, nil
					}
					// Non-connection error (e.g. auth failure)
					fmt.Fprintf(os.Stderr, "%s", output)
					return false, fmt.Errorf("SSH failed: %w", err)
				}
				return true, nil
			})
			if err != nil {
				return err
			}

			// Run the real interactive SSH session
			sshExec := exec.Command("ssh", sshArgs...)
			sshExec.Stdin = os.Stdin
			sshExec.Stdout = os.Stdout
			sshExec.Stderr = os.Stderr

			if err := sshExec.Run(); err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					os.Exit(exitErr.ExitCode())
				}
				return err
			}
			return nil
		},
	}
}

func shouldDisableTTY(extraSSHArgs []string) bool {
	if isTerminal(os.Stdin) {
		return false
	}

	for i := 0; i < len(extraSSHArgs); i++ {
		arg := extraSSHArgs[i]
		switch arg {
		case "-t", "-tt", "-T":
			return false
		}

		if arg == "-o" && i+1 < len(extraSSHArgs) {
			nextArg := strings.TrimSpace(extraSSHArgs[i+1])
			if strings.HasPrefix(nextArg, "RequestTTY=") {
				return false
			}
		}

		if strings.HasPrefix(arg, "-oRequestTTY=") {
			return false
		}
	}

	return true
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
