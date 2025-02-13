package main

import (
	//	"context"
	"encoding/json"
	//	"github.com/EkzikP/sdk-andromeda-go"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	//	"github.com/pkg/errors"
	"log"
	"os"
	"strconv"
)

type (
	config struct {
		TelegramBotToken string            `json:"telegram_bot_token"` //API токен бота
		ApiKey           string            `json:"api_key"`            //API ключ ПО "Центр охраны"
		Host             string            `json:"host"`               //IP адрес сервера ПО "Центр охраны"
		PhoneEngineer    map[string]string `json:"phone_engineer"`     //Список телефонов инженеров ПО "Центр охраны"
	}

	//operation struct {
	//	numberObject   string
	//	currentRequest string
	//	prevMenu       string
	//	checkPanicId   string
	//	changedUserId  string
	//}
)

// readUsers читает файл users.json с ID чата и телефонами пользователей, сохраняет их в map[string]string
func readUsers() *map[string]string {
	file, err := os.Open("users.json")
	if err != nil {
		log.Fatal(err)
	}

	defer func(file *os.File) {
		errClose := file.Close()
		if errClose != nil {
			log.Fatal(errClose)
		}
	}(file)
	tgUser := make(map[string]string)
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&tgUser)
	if err != nil {
		log.Panic(err)
	}
	return &tgUser
}

// readConfig читает файл config.json и сохраняет в структуру config
func readConfig() config {

	file, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}

	defer func(file *os.File) {
		errClose := file.Close()
		if errClose != nil {
			log.Fatal(errClose)
		}
	}(file)

	configuration := config{}
	decoderConfig := json.NewDecoder(file)
	err = decoderConfig.Decode(&configuration)
	if err != nil {
		log.Panic(err)
	}
	return configuration
}

// checkPhone проверяет ввод пользователем номера телефона
func checkPhone(update *tgbotapi.Update, tgUser *map[string]string) bool {

	chatID := strconv.FormatInt(update.Message.Chat.ID, 10)

	if update.Message.Contact == nil {
		if phone, ok := (*tgUser)[chatID]; !ok {
			return false
		} else if len(phone) != 12 {
			return false
		}
		return true
	}

	var contactPhone string
	switch len(update.Message.Contact.PhoneNumber) {
	case 11:
		contactPhone = "+7" + update.Message.Contact.PhoneNumber[1:]
	case 12:
		contactPhone = update.Message.Contact.PhoneNumber
	case 10:
		contactPhone = "+7" + update.Message.Contact.PhoneNumber
	default:
		return false
	}

	if phone, ok := (*tgUser)[chatID]; !ok {
		err := addUser(chatID, contactPhone, tgUser)
		if err != nil {
			return false
		}
		return true
	} else if phone != contactPhone {
		err := addUser(chatID, contactPhone, tgUser)
		if err != nil {
			return false
		}
		return true
	}

	return false
}

// addUser добавляет пользователя в users.json
func addUser(chatID string, phone string, tgUser *map[string]string) error {

	(*tgUser)[chatID] = phone

	file, err := os.Create("users.json")
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		errClose := file.Close()
		if errClose != nil {
			log.Fatal(errClose)
		}
	}(file)
	encoder := json.NewEncoder(file)
	err = encoder.Encode(tgUser)
	if err != nil {
		return err
	}
	return nil
}

// requestPhone запрашивает у пользователя номер телефона
func requestPhone(chatID int64) tgbotapi.MessageConfig {

	button := tgbotapi.KeyboardButton{Text: "Отправить номер телефона", RequestContact: true}
	keyboard := tgbotapi.ReplyKeyboardMarkup{
		Keyboard:        [][]tgbotapi.KeyboardButton{{button}},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
		Selective:       true,
	}

	msg := tgbotapi.NewMessage(chatID, "Oтправьте ваш номер телефона, нажав на кнопку ниже.")
	msg.ReplyMarkup = &keyboard

	return msg
}

func main() {

	//	ctx := context.Background()

	tgUser := *readUsers()
	configuration := readConfig()

	//Создаем структуру с общими параметрами для SDK
	//	confSDK := andromeda.Config{
	//		ApiKey: configuration.ApiKey,
	//		Host:   configuration.Host,
	//	}

	//	currentOperation := make(map[int64]operation)

	bot, err := tgbotapi.NewBotAPI(configuration.TelegramBotToken)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = false

	log.Printf("Авторизация в аккаунте %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {

		var msg tgbotapi.MessageConfig

		chatID := update.Message.Chat.ID

		if update.Message != nil {

			if update.Message.IsCommand() {

				if !checkPhone(&update, &tgUser) {
					msg = requestPhone(chatID)
					msg.ReplyToMessageID = update.Message.MessageID
				} else {
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Введите пультовый номер объекта!")
					msg.ReplyToMessageID = update.Message.MessageID
					//			currentOperation := make(map[int64]operation)
				}
			}

			//Ответ на любое другое сообщение
			if !checkPhone(&update, &tgUser) {
				msg = requestPhone(chatID)
				msg.ReplyToMessageID = update.Message.MessageID
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Введите пультовый номер объекта!")
				msg.ReplyToMessageID = update.Message.MessageID
				//				currentOperation := make(map[int64]operation)
			}
		}

		if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		}

		_, _ = bot.Send(msg)
	}
}
