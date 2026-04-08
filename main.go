package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v3"
)

func main() {
	var path string
	var env string
	var values []string
	var target string
	var dryRun string

	cmd := &cli.Command{
		Name:  "helm-template-diff",
		Usage: "server-side template diffing",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "path",
				Usage:       "Path to helm chart directory",
				Destination: &path,
				Required:    false,
				Validator: func(s string) error {
					entries, err := os.ReadDir(s)
					if err != nil {
						return err
					}

					hasChart := false
					for _, e := range entries {
						if e.Name() == "Chart.yaml" {
							hasChart = true
							break
						}
					}

					if !hasChart {
						return fmt.Errorf("Invalid path - directory does not contain a helm chart")
					}

					return nil
				},
			},
			&cli.StringFlag{
				Name:        "env",
				Usage:       "K8s context (test|production) - maps to values.yaml & `ENV`-values.yaml",
				Destination: &env,
				Required:    false,
				Validator: func(s string) error {
					if s != "test" && s != "production" {
						return fmt.Errorf("ENV value must be one of test|production")
					}

					return nil
				},
			},
			&cli.StringSliceFlag{
				Name:        "values",
				Value:       []string{"values.yaml", "test-values.yaml"},
				Usage:       "If env is not set, uses these explicit values instead",
				Destination: &values,
				Required:    false,
			},
			&cli.StringFlag{
				Name:        "target",
				Value:       "main/master",
				Usage:       "Target branch to compare helm templates with",
				Destination: &target,
				Required:    false,
			},
			&cli.StringFlag{
				Name:        "dry-run",
				Value:       "server",
				Usage:       "Helm template --dry-run argument",
				Destination: &dryRun,
				Required:    false,
			},
		},
		Action: func(ctx context.Context, cliCmd *cli.Command) error {
			if target == "main/master" {
				head, err := getHeadBranch(path)
				if err != nil {
					return err
				}
				target = head
			}
			clean, err := isStatusClean(path)
			if err != nil {
				return err
			}

			if !clean {
				return err
			}

			if path != "" {
				CompareExplicitPath(path, target, env, dryRun)
			} else {
				CompareGitCharts(path, target)
			}

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func CompareGitCharts(target string, path string) error {
	toplevel, err := getTopLevel()
	if err != nil {
		return err
	}

	files, err := getDiffedFiles(target, toplevel)
	if err != nil {
		return err
	}

	uniqueCharts, err := getUniqueCharts(files)
	if err != nil {
		return err
	}
	fmt.Println(uniqueCharts)

	return nil
}

func getTopLevel() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err != nil {
		fmt.Printf("Failed to pipe git rev-parse stdout\n")
		return "", err
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to exec git rev-parse\n")
		return "", err
	}
	str, err := io.ReadAll(stdout)
	if err != nil {
		fmt.Printf("Failed to read git rev-parse stdout\n")
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		fmt.Printf("Failed to exec git rev-parse\n")
		return "", err
	}

	return string(str), nil
}

func CompareExplicitPath(path string, target string, env string, dryRun string) error {
	currTemplate, err := template(path, env, dryRun)
	if err != nil {
		return err
	}

	currBranch, err := getCurrentBranch(path)
	if err != nil {
		fmt.Printf("Failed to get current branch %s\n", path)
		return err
	}

	if err := checkoutBranch(target, path); err != nil {
		fmt.Printf("Failed to checkout target branch %s, %s\n", target, path)
		return err
	}
	headTemplate, err := template(path, env, dryRun)
	if err != nil {
		fmt.Printf("Failed to get target template %s\n", path)
		return err
	}
	if err := checkoutBranch(currBranch, path); err != nil {
		fmt.Printf("Failed to checkout current branch %s, %s\n", currBranch, path)
		return err
	}

	if err := OutputDiffTemplates(headTemplate, currTemplate); err != nil {
		fmt.Printf("Failed to diff files\n")
		return err
	}

	return nil
}

func OutputDiffTemplates(target string, current string) error {
	targetTxt := "target.txt"
	currentTxt := "current.txt"
	if err := os.WriteFile(targetTxt, []byte(target), 0644); err != nil {
		fmt.Printf("Failed to write file %s\n", targetTxt)
		return err
	}
	if err := os.WriteFile(currentTxt, []byte(current), 0644); err != nil {
		fmt.Printf("Failed to write file %s\n", currentTxt)
		return err
	}

	fmt.Println("-------------------------- Server helm template diff --------------------------")

	cmd := exec.Command("diff", "--color=always", "-u", targetTxt, currentTxt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	if err := os.Remove(targetTxt); err != nil {
		fmt.Println(err)
		return err
	}
	if err := os.Remove(currentTxt); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func getCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = path
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	str, err := io.ReadAll(stdout)
	if err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return strings.Trim(string(str), "\n"), nil
}

func getHeadBranch(path string) (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = path
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err != nil {
		fmt.Printf("Failed to pipe git symbolic-ref stdout\n")
		return "", err
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to exec git rev-parse\n")
		return "", err
	}
	str, err := io.ReadAll(stdout)
	if err != nil {
		fmt.Printf("Failed to read git rev-parse stdout\n")
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		fmt.Printf("Failed to exec git rev-parse\n")
		return "", err
	}

	slice := strings.Split(string(str), "/")
	head := strings.Trim(slice[len(slice)-1], "\n")

	return head, nil
}

func checkoutBranch(name string, path string) error {
	cmd := exec.Command("git", "checkout", name)
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Printf("Checked out branch: %s\n", name)

	return nil
}

func isStatusClean(path string) (bool, error) {
	cmd := exec.Command("git", "status")
	cmd.Dir = path
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	str, err := io.ReadAll(stdout)
	if err != nil {
		return false, err
	}
	if err := cmd.Wait(); err != nil {
		return false, err
	}

	clean := strings.Contains(string(str), "nothing to commit, working tree clean")

	if clean {
		return true, nil
	}

	return false, fmt.Errorf("Working tree not clean. Commit or stash all working changes before running helm template diff.")
}

func getDiffedFiles(head string, path string) ([]string, error) {
	cmd := exec.Command("git", "diff", head, "--name-only")
	cmd.Dir = path
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err != nil {
		fmt.Printf("Failed to pipe git diff stdout\n")
		return []string{}, err
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to exec git diff\n")
		return []string{}, err
	}

	namesStr, err := io.ReadAll(stdout)
	if err != nil {
		fmt.Printf("Failed to read git diff pipe\n")
		return []string{}, err
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("Failed to exec git diff\n")
		return []string{}, err
	}

	namesSlice := strings.Split(string(namesStr), "\n")

	return namesSlice, nil
}

// TODO - diff all modified templates, check which Charts the diffed files belong to. Diff each Chart.yaml in that target and current branch
func getUniqueCharts(files []string) ([]string, error) {
	for i := 0; i < len(files); i++ {
		fmt.Println(files[i])
	}
	return files, nil
}

func template(path string, env string, dryRun string) (string, error) {
	args := []string{"template",
		path,
		fmt.Sprintf("-f=%s/values.yaml", path),
		fmt.Sprintf("-f=%s/%s-values.yaml", path, env),
		fmt.Sprintf("--dry-run=%s", dryRun)}
	cmd := exec.Command("helm", args...)
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	str, err := io.ReadAll(stdout)
	if err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	fmt.Printf("Created template for: helm %s\n", args)

	return string(str), nil
}
