// Command dt is a diff traversal tool that follows the named branch of the git
// repo in the current directory, showing the current commit and describing the
// changes made by each commit.
package main

// TODO: support more than 9 changed files
// TODO: support directories

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/mb0/diff"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %v <head-rev>\n", os.Args[0])
		os.Exit(2)
	}
	head := os.Args[1]
	hashes, err := gitLog(head)
	check(err)

	if !gitClean() {
		fmt.Fprintf(os.Stderr, "git tree isn't clean; run git status and resolve\n")
		os.Exit(1)
	}

	defer gitCheckout(head)

	n := 0
	for {
		hash := hashes[n]

		msg, err := gitMessage(hash)
		check(err)

		files, err := gitLs(hash)
		check(err)

		pfiles := make(map[string][]byte)
		if n > 0 {
			pfiles, err = gitLs(hashes[n-1])
			check(err)
		}

		var changed []string
		for name, body := range files {
			if !bytes.Equal(pfiles[name], body) {
				changed = append(changed, name)
			}
		}
		sort.Strings(changed)

		opts := "qr"
		if n > 0 {
			opts += "p"
		}
		if n < len(hashes)-1 {
			opts += "n"
		}
		prompt := bytes.NewBufferString("\x1b[2J\x1b[;H")
		fmt.Fprintf(prompt, "%v\n\n", msg)
		if len(changed) > 0 {
			fmt.Fprintf(prompt, "Changed files:\n")
			for i, name := range changed {
				fmt.Fprintf(prompt, "  [%v] %v\n", i+1, name)
				opts += fmt.Sprintf("%v", i+1)
			}
			fmt.Fprintln(prompt)
		}
		fmt.Fprintf(prompt, "Choice [%v]: ", opts)

		switch c := readChoice(prompt.String(), opts); c {
		case "q":
			return
		case "n":
			n++
			if n >= len(hashes) {
				n = len(hashes) - 1
			}
		case "p":
			n--
			if n < 0 {
				n = 0
			}
		case "r":
			check(gitCheckout(hash))
			check(goRun())
		default:
			i, err := strconv.ParseInt(c, 10, 32)
			check(err)

			name := changed[i-1]
			check(visDiff(pfiles[name], files[name]))
		}
	}
}

func gitClean() bool {
	out, err := exec.Command("git", "status", "-s").CombinedOutput()
	return err == nil && len(out) == 0
}

func goRun() error {
	const bin = "a.out"
	out, err := exec.Command("go", "build", "-o", bin).CombinedOutput()
	if err != nil {
		return fmt.Errorf("goRun build: %v\n%s", err, out)
	}
	defer os.Remove(bin)
	cmd := exec.Command("./" + bin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func readChoice(prompt, opts string) string {
	for {
		fmt.Print(prompt)
		var in string
		fmt.Scan(&in)
		if in == "" {
			continue
		}
		c := in[:1]
		if strings.Index(opts, c) == -1 {
			continue
		}
		return c
	}
}

func gitMessage(rev string) (string, error) {
	out, err := exec.Command("git", "show", "--pretty=format:%B", "-s", rev).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gitMessage %v: %v\n%s", rev, err, out)
	}
	return string(bytes.TrimSpace(out)), nil
}

func gitLs(rev string) (map[string][]byte, error) {
	out, err := exec.Command("git", "ls-tree", rev).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gitLs: %v\n%s", err, out)
	}
	files := make(map[string][]byte)
	for _, s := range strings.Split(string(out), "\n") {
		p := strings.Fields(s)
		if len(p) != 4 {
			continue
		}
		if p[1] != "blob" {
			continue
		}
		out, err := exec.Command("git", "cat-file", "-p", p[2]).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("gitLs cat-file: %v %v\n", p[2], err, out)
		}
		files[p[3]] = out
	}
	return files, nil
}

func gitCheckout(rev string) error {
	out, err := exec.Command("git", "checkout", rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("gitCheckout: %v\n%s", err, out)
	}
	return nil
}

func gitLog(rev string) ([]string, error) {
	cmd := exec.Command("git", "log", "--pretty=format:%H", "--reverse", rev)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func visDiff(a, b []byte) error {
	cs := diff.Bytes(a, b)
	cs = diff.Granular(2, cs) // avoid one-byte chunks

	var buf bytes.Buffer
	n := 0
	for _, c := range cs {
		buf.Write(b[n:c.B])
		if c.Ins > 0 {
			buf.WriteString("\x1b[1;92m")
			buf.Write(b[c.B : c.B+c.Ins])
			buf.WriteString("\x1b[0m")
		}
		n = c.B + c.Ins
	}
	buf.Write(b[n:])

	input := bytes.Replace(buf.Bytes(), []byte("\t"), []byte("        "), -1)

	cmd := exec.Command("less", "-r")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
