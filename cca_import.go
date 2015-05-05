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
)

var (
	abs_dir string
	bucket  *string
	conn    swift.Connection
)

func upload(path string, file_info os.FileInfo, err error) error {
	obj_path := strings.TrimPrefix(path, abs_dir)                     // remove abs_dir from path
	obj_path = strings.TrimPrefix(obj_path, string(os.PathSeparator)) // remove leading slash if it exists
	if len(obj_path) > 0 {
		if file_info.IsDir() {
			err = conn.ObjectPutString(*bucket, obj_path, "", "application/directory")
			if err != nil {
				return err
			}
			fmt.Printf("directory created: %s\n", obj_path)
		} else {
			hash, err := getHash(path)
			if err != nil {
				return err
			}
			obj, _, err := conn.Object(*bucket, obj_path)
			if err != nil {
				return err
			}
			if obj.Hash != hash {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = conn.ObjectPut(*bucket, obj_path, f, true, hash, "", nil)
				if err != nil {
					return err
				}
				fmt.Printf("object uploaded: %s\n", obj_path)
			} else {
				fmt.Printf("object unchanged: %s\n", obj_path)
			}

		}
	}
	return nil
}

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
	dir := flag.String("dir", "", "Directory to be uploaded")
	bucket = flag.String("bucket", "", "The container where the files should be uploaded")
	endpoint := flag.String("endpoint", "https://auth-east.cloud.ca/v2.0", "The Cloud.ca object storage public url")
	identity := flag.String("identity", "", "Your Cloud.ca object storage identity")
	password := flag.String("password", "", "Your Cloud.ca object storage password")
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
	abs_dir, err = filepath.Abs(*dir)
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

	// walk the specified directory and do the upload
	err = filepath.Walk(abs_dir, upload)
	if err != nil {
		fmt.Println("\nERROR: Problem uploading a file\n")
		fmt.Println(err)
		os.Exit(2)
	}
}
