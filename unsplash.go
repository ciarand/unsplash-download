package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"
)

type Image struct {
	Format     string
	Width      int
	Height     int
	Filename   string
	Id         int
	Author     string
	Author_Url string
	Post_Url   string
}

var (
	numWorkers *int
	numRetries *int
	timeout    *int
)

func init() {
	numWorkers = flag.Int("numWorkers", 3, "the number of workers to run at any point")
	numRetries = flag.Int("numRetries", 3, "the number of times to retry a failed download")
	flag.IntVar(numWorkers, "w", 3, "alias for numWorkers")
	flag.IntVar(numWorkers, "r", 3, "alias for numRetries")

	timeout = flag.Int("timeout", 60, "the number of seconds before timing out a worker")
}

func main() {
	// increase the timeout
	http.DefaultClient.Timeout = time.Second * 20

	// a wait group for threads, in case they all timeout
	threadWg := &sync.WaitGroup{}
	// a wait group for images, in case we got 'em all
	imgWg := &sync.WaitGroup{}

	images, err := getImageList()
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	flag.Parse()

	// add the number of images to the img wait group
	imgWg.Add(len(images))
	// number of threads to the thread wait group
	threadWg.Add(*numWorkers)
	// and a buffered channel of images w/ space = num of workers
	in := make(chan Image, *numWorkers)

	// begin the "producer" loop, filling the channel w/ images
	go func() {
		for _, v := range images {
			in <- v
		}
	}()

	// begin the consumer loops
	for i := 0; i < *numWorkers; i += 1 {
		go downloadLoop(in, imgWg, threadWg)
	}

	// wait for either the thread wait group or the image wait group

	done := make(chan bool)

	go func() {
		threadWg.Wait()
		done <- true
	}()
	go func() {
		imgWg.Wait()
		done <- true
	}()

	<-done
}

func (image *Image) Download() error {
	// if we've already got it, don't re-get it
	if _, err := os.Stat("images/" + image.Filename); err == nil {
		return nil
	}

	res, err := http.Get(image.Post_Url + "/download")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile("images/"+image.Filename, body, 0664)
	if err != nil {
		return err
	}

	fmt.Printf("done downloading %s\n", image.Filename)
	return nil
}

func getImageList() ([]Image, error) {
	var images []Image

	// GET all of our JSON
	res, err := http.Get("https://unsplash.it/list")
	if err != nil {
		return nil, fmt.Errorf("Couldn't retrieve unsplash.it list: %s", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Couldn't read the unsplash.it list: %s", err)
	}

	if err := json.Unmarshal(body, &images); err != nil {
		return nil, fmt.Errorf("Couldn't unmarshal json (%s): %s", string(body), err)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("images collection is empty: %s", string(body))
	}

	return images, nil
}

func downloadLoop(conns chan Image, imgWg, threadWg *sync.WaitGroup) {
	for {
		select {
		case img := <-conns:
			// and try 3 times to download a file
			for tries := 0; tries < *numRetries; tries += 1 {
				if err := img.Download(); tries == (*numRetries-1) && err != nil {
					fmt.Printf("Couldn't download %s: %s\n", img.Filename, err)
				} else if err == nil {
					// successful get
					break
				}
			}

			imgWg.Done()

		case <-time.After(time.Duration(*timeout) * time.Second):
			threadWg.Done()
			return
		}
	}
}
