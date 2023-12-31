package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"github.com/spf13/viper"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type TgBotConfig struct {
	FileDir    string `mapstructure:"fileDir"`
	TGBotToken string `mapstructure:"TG_BOT_TOKEN"`
	TGChatID   int64  `mapstructure:"TG_CHAT_ID"`
	ServerPort int    `mapstructure:"server_port"`
}

func main() {
	viper.SetConfigFile("config_bot.toml")
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Println("Failed to read config file:", err)
		return
	}
	var config TgBotConfig
	err = viper.Unmarshal(&config)
	if err != nil {
		fmt.Println("Failed to unmarshal config file:", err)
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go startTgBot(&wg, config.FileDir, config.TGBotToken, config.TGChatID)
	go startServer(&wg, config.FileDir, config.ServerPort)
	wg.Wait()
}





func handleFiles(saveDir string) {
	// 指定目录
	// 删除创建时间在4小时之前的文件
	deleteOldFiles(saveDir)
	// 合并剩余的txt文件到all.txt中
	mergeTxtFiles(saveDir)
	log.Println("Files processed successfully!")
}

func deleteFiles(dirPath string) (int, error) {
	count := 0
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			err := os.Remove(path)
			if err == nil {
				count++
			}
		}
		return nil
	})
	return count, err
}

// 删除创建时间在4小时之前的文件
func deleteOldFiles(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	currentTime := time.Now()
	thresholdTime := currentTime.Add(-4 * time.Hour)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if file.ModTime().Before(thresholdTime) {
			err := os.Remove(filepath.Join(dir, file.Name()))
			if err != nil {
				log.Printf("Failed to delete file %s: %s\n", file.Name(), err)
			} else {
				log.Printf("Deleted file: %s\n", file.Name())
			}
		}
	}
}

// 将剩余的txt文件合并到all.txt中
func mergeTxtFiles(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	outputFile, err := os.Create(filepath.Join(dir, "all.txt"))
	if err != nil {
		log.Fatal(err)
	}
	defer outputFile.Close()

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".txt" {
			continue
		}

		inputFile, err := os.Open(filepath.Join(dir, file.Name()))
		if err != nil {
			log.Printf("Failed to open file %s: %s\n", file.Name(), err)
			continue
		}

		content, err := ioutil.ReadAll(inputFile)
		if err != nil {
			log.Printf("Failed to read file %s: %s\n", file.Name(), err)
			inputFile.Close()
			continue
		}

		if _, err := outputFile.Write(content); err != nil {
			log.Printf("Failed to write file %s to all.txt: %s\n", file.Name(), err)
		}

		inputFile.Close()
		log.Printf("Merged file: %s\n", file.Name())
	}

	log.Println("Files merged successfully!")
}

//tg bot

func startTgBot(wg *sync.WaitGroup, fileSaveDir string, TG_TOKEN string, CHAT_ID int64) {
	defer wg.Done()
	// 替换为你自己的Telegram Bot Token
	bot, err := tgbotapi.NewBotAPI(TG_TOKEN)
	if err != nil {
		log.Fatal(err)
	}

	// 设置机器人的更新频率
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// 获取机器人的更新通道
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
	}

	// 创建保存文件的目录（如果不存在）
	err = os.MkdirAll(fileSaveDir, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	// 监听机器人的更新
	for update := range updates {
		// 处理接收到的消息
		if update.Message != nil {
			// 检查消息是否来自指定的聊天
			if update.Message.Chat.ID == CHAT_ID {
				// 检查消息是否包含文件
				if update.Message.Document != nil {
					// 获取文件信息
					fileID := update.Message.Document.FileID
					fileName := update.Message.Document.FileName

					// 检查文件是否以.txt结尾
					if strings.HasSuffix(fileName, ".txt") {
						// 下载文件
						filePath, err := downloadFile(bot, fileID, fileSaveDir)
						if err != nil {
							log.Println("Failed to download file:", err)
							continue
						}
						log.Println("File saved:", filePath)
						handleFiles(fileSaveDir)
					}
				}
			}
			if update.Message.IsCommand() && update.Message.Command() == "clear" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
				msg.ReplyToMessageID = update.Message.MessageID
				count, err := deleteFiles(fileSaveDir)
				if err != nil {
					msg.Text = "删除文件时发生错误"
				} else {
					msg.Text = fmt.Sprintf("已完成删除 %d 个文件", count)
				}
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}
}

// 下载文件并保存到指定目录
func downloadFile(bot *tgbotapi.BotAPI, fileID string, saveDir string) (string, error) {
	file, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", err
	}

	// 根据文件ID生成保存文件的路径
	filePath := fmt.Sprintf("%s/%d_%s.txt", saveDir, time.Now().Unix(), file.FileID)

	// 创建保存文件的文件
	out, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {

		}
	}(out)

	// 获取文件的直接URL
	url, err := bot.GetFileDirectURL(file.FileID)
	log.Println("File download url-> ", url)
	if err != nil {
		return "", err
	}

	// 下载文件内容
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 将文件内容复制到保存文件中
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return filePath, nil
}

//http server

const (
	allFilePath   = "all.txt"
	validFilePath = "valid.txt"
)

func startServer(wg *sync.WaitGroup, fileDir string, port int) {
	// 创建一个文件服务器处理器
	defer wg.Done()
	http.HandleFunc("/all", func(w http.ResponseWriter, r *http.Request) {
		// 读取指定目录下的文件
		filePath := filepath.Join(fileDir, "all.txt")
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			http.Error(w, "Failed to read all.txt", http.StatusInternalServerError)
			return
		}
		w.Write(content)
	})

	http.HandleFunc("/valid", handleValid)

	fmt.Println(fmt.Sprintf("Server is running on http://localhost:%d", port))
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func handleValid(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filePath := filepath.Join("uploads", validFilePath)
	err = os.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	f, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	_, err = io.Copy(f, file)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "File uploaded and saved successfully")
}
