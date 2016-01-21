package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"github.com/ncw/swift"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Path struct {
	file_path string
	obj_path  string
	dir       bool
}

var (
	conn       swift.Connection
	dir        = flag.String("dir", "", "Absolute or relative path to a directory to be uploaded")
	bucket     = flag.String("bucket", "", "The container where the files should be uploaded")
	endpoint   = flag.String("endpoint", "https://auth-east.cloud.ca/v2.0", "The Cloud.ca object storage public url")
	identity   = flag.String("identity", "", "Your Cloud.ca object storage identity")
	password   = flag.String("password", "", "Your Cloud.ca object storage password")
	prefix     = flag.String("prefix", "", "A prefix added to the path of each object uploaded to the bucket")
	concurrent = flag.Int("concurrent", 4, "The number of files to be uploaded concurrently (reduce if 'too many files open' errors occur)")
)

func getHash(file_path string) (string, error) {
	f, err := os.Open(file_path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func main() {
	var err error
	flag.Parse()

	// make sure we have all the details we need
	if *dir == "" || *bucket == "" || *identity == "" || *password == "" {
		fmt.Println("\nERROR: 'dir', 'bucket', 'identity' and 'password' are required\n")
		flag.Usage()
		os.Exit(2)
	}

	// get the details about the identity (tenant and user)
	parts := strings.Split(*identity, ":")
	var tenant, username string
	if len(parts) > 1 {
		tenant = parts[0]
		username = parts[1]
	} else {
		fmt.Println("\nERROR: The 'identity' needs to be formated as '<tenant>:<username>'\n")
		flag.Usage()
		os.Exit(2)
	}

	// make 'dir' absolute so it is easier to work with
	abs_dir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Println("\nERROR: Problem resolving the specified directory\n")
		os.Exit(2)
	}

	// make a swift connection
	conn = swift.Connection{
		Tenant:   tenant,
		UserName: username,
		ApiKey:   *password,
		AuthUrl:  *endpoint,
	}

	// authenticate swift user
	err = conn.Authenticate()
	if err != nil {
		fmt.Println("\nERROR: Authentication failed.  Validate your credentials are correct\n")
		os.Exit(2)
	}

	// create the container if it does not already exist
	err = conn.ContainerCreate(*bucket, nil)
	if err != nil {
		fmt.Println("\nERROR: Problem creating the specified bucket")
		fmt.Println(err)
		os.Exit(2)
	}
	fmt.Printf("Using bucket: %s\n", *bucket)
	fmt.Println("Starting upload...  This can take a while, go get a coffee.  :)")

	// setup and get paths in the 'dir'
	paths := make([]*Path, 0)
	// create the 'prefix' directory structure for the objects
	pre_path := strings.Trim(*prefix, string(os.PathSeparator))
	pre_path_parts := strings.Split(pre_path, string(os.PathSeparator))
	pre_dirs := "" // will result in the final dir structure to add to each object
	for i := 0; i < len(pre_path_parts); i++ {
		if pre_dirs == "" { // initialize the path
			pre_dirs = pre_path_parts[i]
		} else { // append the next part to the path
			pre_dirs = strings.Join([]string{pre_dirs, pre_path_parts[i]}, "/")
		}
		// append this segment of the prefix path to the list of paths
		// we add each directory level as its own directory
		paths = append(paths, &Path{
			obj_path: pre_dirs,
			dir:      true,
		})
	}
	// filepath.Walk() is a blocking function, so keep it as simple as possible
	err = filepath.Walk(abs_dir, func(path string, info os.FileInfo, _ error) (err error) {
		if info.IsDir() {
			paths = append(paths, &Path{
				file_path: path,
				dir:       true,
			})
		} else {
			if info.Mode().IsRegular() {
				paths = append(paths, &Path{
					file_path: path,
					dir:       false,
				})
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println("\nERROR: Problem discovering a file\n")
		fmt.Println(err)
		os.Exit(2)
	}

	// now upload all the objects into the established dirs
	process_path := func(p *Path) error {
		// setup the p.obj_path to represent the path it should have in the object store
		if p.obj_path == "" && p.file_path != "" {
			p.obj_path = strings.TrimPrefix(p.file_path, abs_dir)                 // remove abs_dir from path
			p.obj_path = strings.TrimPrefix(p.obj_path, string(os.PathSeparator)) // remove leading slash if it exists
			p.obj_path = filepath.ToSlash(p.obj_path)                             // fix windows paths
			if len(p.obj_path) > 0 && len(pre_dirs) > 0 {
				p.obj_path = strings.Join([]string{pre_dirs, p.obj_path}, "/") // prepend the prefix
			}
		}

		if len(p.obj_path) > 0 {
			switch {
			case p.dir: // dir structure in object store
				obj, _, err := conn.Object(*bucket, p.obj_path)
				if err == nil && obj.ContentType == "application/directory" {
					fmt.Printf("unchanged: %s\n", p.obj_path)
				} else {
					err = conn.ObjectPutString(*bucket, p.obj_path, "", "application/directory")
					if err != nil {
						fmt.Printf("\nERROR: Problem creating folder '%s'\n", p.obj_path)
						fmt.Println(err)
						return err
					}
					fmt.Printf("added dir: %s\n", p.obj_path)
				}
			case !p.dir: // file object to be uploaded
				hash, err := getHash(p.file_path)
				if err != nil {
					fmt.Printf("\nERROR: Problem creating object hash\n")
					fmt.Println(err)
					return err
				}
				obj, _, err := conn.Object(*bucket, p.obj_path)
				if err != nil || obj.Hash != hash { // if error or hash mismatch
					fmt.Printf("  started: %s\n", p.obj_path)
					f, err := os.Open(p.file_path)
					if err != nil {
						fmt.Printf("\nERROR: Problem opening file '%s'\n", p.file_path)
						fmt.Println(err)
						return err
					}
					defer f.Close()
					_, err = conn.ObjectPut(*bucket, p.obj_path, f, true, hash, "", nil)
					if err != nil {
						fmt.Printf("\nERROR: Problem uploading object '%s'\n", p.obj_path)
						fmt.Println(err)
						return err
					}
					fmt.Printf(" uploaded: %s\n", p.obj_path)
				} else {
					fmt.Printf(" unchanged: %s\n", p.obj_path)
				}
			}
		}

		return nil
	}

	// setup 'process_path' concurrency controls
	pathc := make(chan *Path)
	var wg sync.WaitGroup
	// setup the number of concurrent goroutine workers
	for i := 0; i < *concurrent; i++ {
		wg.Add(1)
		go func() {
			for p := range pathc {
				process_path(p)
			}
			wg.Done()
		}()
	}
	// feed the paths into the concurrent goroutines to be executed
	for _, p := range paths {
		pathc <- p
	}
	close(pathc)
	wg.Wait()
}
