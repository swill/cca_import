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
}

var (
	conn swift.Connection
)

func getHash(path string) (string, error) {
	f, err := os.Open(path)
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
	dir := flag.String("dir", "", "Absolute or relative path to a directory to be uploaded")
	bucket := flag.String("bucket", "", "The container where the files should be uploaded")
	endpoint := flag.String("endpoint", "https://auth-east.cloud.ca/v2.0", "The Cloud.ca object storage public url")
	identity := flag.String("identity", "", "Your Cloud.ca object storage identity")
	password := flag.String("password", "", "Your Cloud.ca object storage password")
	prefix := flag.String("prefix", "", "A prefix added to the path of each object uploaded to the bucket")
	concurrent := flag.Int("concurrent", 4, "The number of files to be uploaded concurrently (reduce if 'too many files open' errors occur)")
	flag.Parse()

	if *dir == "" || *bucket == "" || *identity == "" || *password == "" {
		fmt.Println("\nERROR: 'dir', 'bucket', 'identity' and 'password' are required\n")
		flag.Usage()
		os.Exit(2)
	}

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

	// make dir absolute so it is easier to work with
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

	// walk the file system and pull out the important info (because 'Walk' is a blocking function)
	dirs := make([]*Path, 0)
	objs := make([]*Path, 0)
	pre_path := strings.Trim(*prefix, string(os.PathSeparator))
	pre_path_parts := strings.Split(pre_path, string(os.PathSeparator))
	pre_dirs := ""
	for i := 0; i < len(pre_path_parts); i++ {
		if pre_dirs == "" {
			pre_dirs = pre_path_parts[i]
		} else {
			pre_dirs = strings.Join([]string{pre_dirs, pre_path_parts[i]}, "/")
		}
		dirs = append(dirs, &Path{
			obj_path: pre_dirs,
		})
	}
	err = filepath.Walk(abs_dir, func(path string, info os.FileInfo, _ error) (err error) {
		obj_path := strings.TrimPrefix(path, abs_dir)                     // remove abs_dir from path
		obj_path = strings.TrimPrefix(obj_path, string(os.PathSeparator)) // remove leading slash if it exists
		if len(obj_path) > 0 {
			if pre_path != "" {
				obj_path = strings.Join([]string{pre_path, obj_path}, string(os.PathSeparator))
			}
			obj_path = filepath.ToSlash(obj_path) // fix windows paths
			if info.IsDir() {
				dirs = append(dirs, &Path{
					obj_path: obj_path,
				})
			} else {
				if info.Mode().IsRegular() {
					objs = append(objs, &Path{
						file_path: path,
						obj_path:  obj_path,
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println("\nERROR: Problem discovering a file\n")
		fmt.Println(err)
		os.Exit(2)
	}

	// put all the dirs in place initially
	var dir_wg sync.WaitGroup
	for _, p := range dirs {
		dir_wg.Add(1)
		go func(obj_path string) error {
			defer dir_wg.Done()
			if obj_path != "" {
				obj, _, err := conn.Object(*bucket, obj_path)
				if err == nil && obj.ContentType == "application/directory" {
					fmt.Printf("unchanged: %s\n", obj_path)
				} else {
					err = conn.ObjectPutString(*bucket, obj_path, "", "application/directory")
					if err != nil {
						fmt.Printf("\nERROR: Problem creating folder '%s'\n", obj_path)
						fmt.Println(err)
						return err
					}
					fmt.Printf("added dir: %s\n", obj_path)
				}
			}
			return nil
		}(p.obj_path)
	}
	dir_wg.Wait()

	// now upload all the objects into the established dirs
	process_path := func(path, obj_path string) error {
		hash, err := getHash(path)
		if err != nil {
			fmt.Printf("\nERROR: Problem creating object hash\n")
			fmt.Println(err)
			return err
		}
		obj, _, err := conn.Object(*bucket, obj_path)
		if err != nil || obj.Hash != hash {
			fmt.Printf("  started: %s\n", obj_path)
			f, err := os.Open(path)
			if err != nil {
				fmt.Printf("\nERROR: Problem opening file '%s'\n", path)
				fmt.Println(err)
				return err
			}
			defer f.Close()
			_, err = conn.ObjectPut(*bucket, obj_path, f, true, hash, "", nil)
			if err != nil {
				fmt.Printf("\nERROR: Problem uploading object '%s'\n", obj_path)
				fmt.Println(err)
				return err
			}
			fmt.Printf(" uploaded: %s\n", obj_path)
		} else {
			fmt.Printf(" unchanged: %s\n", obj_path)
		}
		return nil
	}

	// setup 'process_path' concurrency controls
	pathc := make(chan *Path)
	var obj_wg sync.WaitGroup
	// setup the number of concurrent goroutine workers
	for i := 0; i < *concurrent; i++ {
		obj_wg.Add(1)
		go func() {
			for p := range pathc {
				process_path(p.file_path, p.obj_path)
			}
			obj_wg.Done()
		}()
	}
	// feed the paths into the concurrent goroutines to be executed
	for _, p := range objs {
		pathc <- p
	}
	close(pathc)
	obj_wg.Wait()
}
