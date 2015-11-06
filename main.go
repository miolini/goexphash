package main

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
)

var (
	newLine       = "\n"
	newLineDouble = "\n\n"

	flVerbose         = flag.Bool("v", false, "verbose mode")
	flPrintDescriptor = flag.Bool("p", false, "print descriptor")
	flDownloadPackage = flag.Bool("d", false, "run go get tool")
)

func main() {
	if *flVerbose {
		log.Print("GoExpHash - exported symbols hash calculator")
	}
	flag.Parse()
	args := flag.Args()
	if flag.NArg() != 1 {
		log.Fatal("usage: goexphash <package name>")
	}
	packageName := args[len(args)-1]
	if *flVerbose {
		log.Printf("hash package: %s", packageName)
	}
	localHash, _, err := hashPackage(packageName)
	if err != nil {
		log.Fatalf("error: %s", err)
	}
	fmt.Println(localHash)
}

func runCmd(execCmd string, args ...string) error {
	cmd := exec.Command(execCmd, args...)
	if *flVerbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func lookupPackagePath(packageName string) (path string, err error) {
	if *flVerbose {
		log.Printf("lookup package path: %s", packageName)
	}
	gopath, ok := os.LookupEnv("GOPATH")
	if !ok {
		err = fmt.Errorf("GOPATH not found")
		return
	}
	gopathDirs := strings.Split(gopath, ":")

	if *flDownloadPackage {
		err = runCmd("go", "get", "-u", "-v", packageName)
		if err != nil {
			err = fmt.Errorf("go get err: %s", err)
			return
		}
		path = gopathDirs[0] + "/src/" + packageName
		return
	}

	for _, gopathDir := range gopathDirs {
		tmppath := gopathDir + "/src/" + packageName
		if ok, err = exists(tmppath); err != nil {
			err = fmt.Errorf("check exists path '%s' error: %s", tmppath, err)
			return
		} else if ok {
			path = tmppath
			return
		}
	}
	return
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

type exportItems []string

func (e exportItems) Len() int {
	return len(e)
}

func (e exportItems) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e exportItems) Less(i, j int) bool {
	return e[i] < e[j]
}

func hashPackage(packageName string) (localHash, globalHash string, err error) {
	localBuf := bytes.Buffer{}
	globalBuf := bytes.Buffer{}
	packagePath, err := lookupPackagePath(packageName)
	if err != nil {
		return
	}
	if *flVerbose {
		log.Printf("package path: %s", packagePath)
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, packagePath, nil, 0)
	if err != nil {
		return
	}
	items := exportItems{}
	imports := map[string]bool{}
	for _, pkg := range pkgs {
		for fileName, file := range pkg.Files {
			if strings.HasSuffix(fileName, "_test.go") {
				continue
			}
			ast.FileExports(file)
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncDecl:
					fn := getFuncSignature(fset, x)
					items = append(items, fn)
				case *ast.GenDecl:
					s := sprintNode(fset, x)
					if strings.HasPrefix(s, "const (") {
						s = strings.Replace(s, newLineDouble, newLine, -1)
						parts := strings.Split(s, newLine)
						for i := 1; i < len(parts)-1; i++ {
							part := removeSpace(parts[i])
							items = append(items, "const "+part)
						}
					} else if strings.HasPrefix(s, "var (") {
						parts := strings.Split(s, newLine)
						for i := 1; i < len(parts)-1; i++ {
							part := removeSpace(parts[i])
							items = append(items, "var "+part)
						}
					} else if strings.HasPrefix(s, "type (") {
						parts := strings.Split(s, newLineDouble)
						for i := 1; i < len(parts)-1; i++ {
							part := removeSpace(parts[i])
							items = append(items, "type "+part)
						}
					} else {
						items = append(items, s)
					}
				case *ast.ImportSpec:
					s := sprintNode(fset, x.Path)
					imports[s] = true
				}
				return true
			})
		}
	}
	sort.Sort(items)
	for _, item := range items {
		if *flPrintDescriptor {
			log.Printf("%s", item)
		}
		localBuf.Write([]byte(item))
		localBuf.Write([]byte(newLine))
	}
	localHash = sha512String(localBuf)
	globalHash = sha512String(globalBuf)
	return
}

func removeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func getFuncSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	buf := bytes.Buffer{}
	printer.Fprint(&buf, fset, fn)
	data := buf.Bytes()
	return string(data[:bytes.IndexByte(data, '\n')-2])
}

func sprintNode(fset *token.FileSet, n ast.Node) string {
	buf := bytes.Buffer{}
	printer.Fprint(&buf, fset, n)
	return string(buf.Bytes())
}

func sha512String(buf bytes.Buffer) string {
	hash := sha512.New512_256()
	hash.Write(buf.Bytes())
	return hex.EncodeToString(hash.Sum(nil))
}
