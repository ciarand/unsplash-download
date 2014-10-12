package main

import (
	"encoding/json"
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

func Download(image Image) error {
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

func main() {
	// increase the timeout
	http.DefaultClient.Timeout = time.Second * 20

	// GET all of our JSON
	res, err := http.Get("https://unsplash.it/list")
	if err != nil {
		fmt.Print("Couldn't retrieve unsplash.it list:", err)
		os.Exit(1)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Print("Couldn't read the unsplash.it list:", err)
		os.Exit(1)
	}

	var images []Image
	if err = json.Unmarshal(body, &images); err != nil {
		fmt.Printf("Couldn't unmarshal json (%s): %s", string(body), err)
		os.Exit(1)
	}

	if len(images) == 0 {
		fmt.Printf("images collection is empty: %s", string(body))
		os.Exit(1)
	}

	max := 3
	imgWg := &sync.WaitGroup{}
	imgWg.Add(len(images))

	threadWg := &sync.WaitGroup{}
	threadWg.Add(max)

	conns := make(chan Image, max)

	// begin the "producer" loop
	go func() {
		for _, v := range images {
			conns <- v
		}
	}()

	// begin the consumer loop w/ a total of 10 gothreads
	for i := 0; i < max; i += 1 {
		go func(num int) {
			// each of them will loop and block on receive
			for {
				select {
				case img := <-conns:
					// and try 3 times to download a file
					for tries := 0; tries < 3; tries += 1 {
						if err := Download(img); tries == 2 && err != nil {
							fmt.Printf("Couldn't download %s: %s\n", img.Filename, err)
						} else if err == nil {
							// successful get
							break
						}
					}

					imgWg.Done()

				case <-time.After(1 * time.Minute):
					fmt.Printf("worker thread #%d done\n", num)
					threadWg.Done()
					return
				}
			}
		}(i)
	}

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
