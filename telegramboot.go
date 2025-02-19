package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/EkzikP/sdk-andromeda-go"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pkg/errors"
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

	operation struct {
		numberObject   string
		object         andromeda.GetSitesResponse
		customers      []andromeda.GetCustomerResponse
		currentRequest string
		prevMenu       string
		checkPanicId   string
		changedUserId  string
	}

	menu struct {
		text         string
		callbackData string
	}
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

	msg := tgbotapi.NewMessage(chatID, "Отправьте ваш номер телефона, нажав на кнопку ниже.")
	msg.ReplyMarkup = &keyboard

	return msg
}

// checkNumberObject проверяет ввод пользователем номера объекта
func checkNumberObject(text string) (string, bool) {

	num, err := strconv.Atoi(text)
	if err != nil {
		return "Номер объекта введен некорректно!", false
	}

	if num < 1 || num > 9999 {
		return "Номер объекта введен некорректно!", false
	}
	return "", true
}

// findObject получает объект по номеру
func findObject(numberObject string, confSDK andromeda.Config, client *andromeda.Client, ctx *context.Context) (andromeda.GetSitesResponse, error) {

	getSiteRequest := andromeda.GetSitesInput{
		Id:     numberObject,
		Config: confSDK,
	}

	getSiteResponse, err := client.GetSites(*ctx, getSiteRequest)
	if err != nil {
		return andromeda.GetSitesResponse{}, err
	}
	return getSiteResponse, nil
}

// checkUserRights проверяет права пользователя
func checkUserRights(object andromeda.GetSitesResponse, currentOperation *map[int64]operation, chatID int64, confSDK andromeda.Config, tgUser *map[string]string, phoneEngineer map[string]string, client *andromeda.Client, ctx *context.Context) bool {

	getCustomersRequest := andromeda.GetCustomersInput{
		SiteId: object.Id,
		Config: confSDK,
	}

	getCustomersResponse, err := client.Customers(*ctx, getCustomersRequest)
	if err != nil {
		return false
	}

	var useRights bool
	phoneUser := (*tgUser)[strconv.FormatInt(chatID, 10)]
	for _, customer := range getCustomersResponse {
		var phoneCustomer string
		switch len(customer.ObjCustPhone1) {
		case 12:
			phoneCustomer = customer.ObjCustPhone1
		case 11:
			phoneCustomer = "+7" + customer.ObjCustPhone1[1:]
		case 10:
			phoneCustomer = "+7" + customer.ObjCustPhone1
		default:
			phoneCustomer = ""
		}
		if phoneUser == phoneCustomer {
			useRights = true
			break
		}
	}

	if !useRights && !isEngineer(phoneUser, phoneEngineer) {
		return false
	}

	(*currentOperation)[chatID] = operation{
		numberObject:   strconv.Itoa(object.AccountNumber),
		object:         object,
		customers:      getCustomersResponse,
		currentRequest: "",
		prevMenu:       "",
		checkPanicId:   "",
		changedUserId:  "",
	}
	return true
}

// checkPhone проверяет права инженера
func isEngineer(phone string, phoneEngineer map[string]string) bool {
	if _, ok := phoneEngineer[phone]; ok {
		return true
	}
	return false
}

// createMainMenu создает меню
func createMenu(chatId int64, operation operation) tgbotapi.MessageConfig {

	msg := createMainMenu(chatId, operation)
	return msg
}

// createMainMenu создает главное меню
func createMainMenu(chatID int64, operation operation) tgbotapi.MessageConfig {
	mainMenu := []menu{
		{"Получить список ответственных лиц объекта", "GetCustomers"},
		{"Проверка КТС", "ChecksKTS"},
		{"Управление доступом в MyAlarm (в работе)", "MyAlarm"},
		{"Операции с карточкой объектов (не сделано)", "Object"},
		{"Управление разделами объекта (не сделано)", "Sections"},
		{"Управление шлейфами (не сделано)", "Shift"},
		{"Завершить работу с объектом", "Finish"},
	}

	keyboard := tgbotapi.InlineKeyboardMarkup{}
	for _, button := range mainMenu {
		var row []tgbotapi.InlineKeyboardButton
		btn := tgbotapi.NewInlineKeyboardButtonData(button.text, button.callbackData)
		row = append(row, btn)
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
	}
	text := fmt.Sprintf("Работа с объектом %s!\nВыберите пункт меню:", operation.numberObject)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = &keyboard
	return msg
}

// addBtnBack добавляет кнопку назад к сообщениям
func addButtons(currentRequest string, err error) tgbotapi.InlineKeyboardMarkup {

	keyboard := tgbotapi.NewInlineKeyboardMarkup()

	if currentRequest == "ChecksKTS" && err == nil {
		btnKTS := tgbotapi.NewInlineKeyboardButtonData("Получить результат проверки КТС", "ResultCheckKTS")
		var rowKTS []tgbotapi.InlineKeyboardButton
		rowKTS = append(rowKTS, btnKTS)
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, rowKTS)
	}

	btn := tgbotapi.NewInlineKeyboardButtonData("Назад", "Back")
	var row []tgbotapi.InlineKeyboardButton
	row = append(row, btn)
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)

	btnFinish := tgbotapi.NewInlineKeyboardButtonData("Завершить работу с объектом", "Finish")
	var rowFinish []tgbotapi.InlineKeyboardButton
	rowFinish = append(rowFinish, btnFinish)
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, rowFinish)

	return keyboard
}

// checksKTSRequest проверка КТС
func checksKTSRequest(currentOperation *map[int64]operation, chatID int64, confSDK andromeda.Config, client *andromeda.Client, ctx context.Context) tgbotapi.MessageConfig {

	PostCheckPanicRequest := andromeda.PostCheckPanicInput{
		SiteId: (*currentOperation)[chatID].object.Id,
		Config: confSDK,
	}
	PostCheckPanicResponse, err := client.PostCheckPanic(ctx, PostCheckPanicRequest)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "Не удалось получить данные")
		msg.ReplyMarkup = addButtons((*currentOperation)[chatID].currentRequest, err)
		return msg
	}

	PostCheckPanic := map[string]string{
		"has alarm":                   "по объекту есть тревога, проверка КТС запрещена",
		"already run":                 "по объекту уже выполняется проверка КТС",
		"success":                     "проверка КТС начата",
		"error":                       "при выполнении запроса произошла ошибка",
		"invalid checkInterval value": "для параметра checkInterval задано значение, выходящее за пределы допустимого диапазона",
	}

	if PostCheckPanicResponse.Description != "success" {
		msg := tgbotapi.NewMessage(chatID, PostCheckPanic[PostCheckPanicResponse.Description])
		err = errors.New(PostCheckPanic[PostCheckPanicResponse.Description])
		msg.ReplyMarkup = addButtons((*currentOperation)[chatID].currentRequest, err)
		return msg
	}

	(*currentOperation)[chatID] = operation{
		numberObject:   (*currentOperation)[chatID].numberObject,
		object:         (*currentOperation)[chatID].object,
		customers:      (*currentOperation)[chatID].customers,
		currentRequest: (*currentOperation)[chatID].currentRequest,
		prevMenu:       (*currentOperation)[chatID].prevMenu,
		checkPanicId:   PostCheckPanicResponse.CheckPanicId,
		changedUserId:  (*currentOperation)[chatID].changedUserId,
	}
	text := fmt.Sprintf("%s\nВ течении 180 сек. нажмите кнпку КТС.\nИ нажмите кнопку \"Получить результат проверки КТС\"", PostCheckPanic[PostCheckPanicResponse.Description])
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = addButtons((*currentOperation)[chatID].currentRequest, nil)
	return msg

	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		if update.CallbackQuery.Data == "ResultCheckKTS" {
			GetCheckPanicRequest := andromeda.GetCheckPanicInput{
				CheckPanicId: (*currentOperation)[chatID].checkPanicId,
				Config:       apiHost,
			}

			id := (*currentOperation)[chatID].numberObject

			client, _, err := validateUser(ctx, *tgUser, configuration, chatID, id, apiHost)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, err.Error())
				msg.ReplyMarkup = addBtnBack()
				return msg
			}

			GetCheckPanicResponse, err := client.GetCheckPanic(ctx, GetCheckPanicRequest)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, err.Error())
				msg.ReplyMarkup = addBtnBack()
				return msg
			}

			CheckPanicResponse := map[string]string{
				"not found":                   "проверка с КТС не найдена",
				"in progress":                 "проверка КТС продолжается (не завершена): КТС не получена, тайм-аут не истек",
				"success":                     "проверка КТС успешно завершена",
				"success, interval continues": "проверка КТС успешно завершена, но продолжается отсчет интервала проверки",
				"time out":                    "проверка КТС завершена с ошибкой: истек интервал ожидания события КТС",
				"error":                       "при выполнении запроса произошла ошибка",
			}
			msg := tgbotapi.NewMessage(chatID, CheckPanicResponse[GetCheckPanicResponse.Description])
			msg.ReplyMarkup = addBtnBack()
			return msg
		}
	}

	msg := tgbotapi.NewMessage(chatID, "Введите номер объекта для получения информации")
	msg.ReplyMarkup = addBtnBack()
	return msg
}

func main() {

	ctx := context.Background()

	tgUser := *readUsers()
	configuration := readConfig()

	//Создаем структуру с общими параметрами для SDK
	confSDK := andromeda.Config{
		ApiKey: configuration.ApiKey,
		Host:   configuration.Host,
	}

	currentOperation := make(map[int64]operation)

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

		client := andromeda.NewClient()

		if update.Message != nil {

			chatID := update.Message.Chat.ID

			if update.Message.Chat.IsPrivate() {

				if update.Message.IsCommand() {

					if !checkPhone(&update, &tgUser) {
						msg = requestPhone(chatID)
						msg.ReplyToMessageID = update.Message.MessageID
					} else {
						currentOperation[chatID] = operation{}
						msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Введите пультовый номер объекта!")
						msg.ReplyToMessageID = update.Message.MessageID
					}
				} else {
					//Проверка номера объекта и прав пользователя для работы с этим объектом
					if currentOperation[chatID].numberObject == "" {
						if !checkPhone(&update, &tgUser) {
							msg = requestPhone(chatID)
							msg.ReplyToMessageID = update.Message.MessageID
						} else if message, ok := checkNumberObject(update.Message.Text); !ok {
							text := fmt.Sprintf("%s\nВведите пультовый номер объекта!", message)
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
							msg.ReplyToMessageID = update.Message.MessageID
						} else if object, err := findObject(update.Message.Text, confSDK, client, &ctx); err != nil {
							text := fmt.Sprintf("%s\nВведите пультовый номер объекта!", err)
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
							msg.ReplyToMessageID = update.Message.MessageID
						} else if !checkUserRights(object, &currentOperation, chatID, confSDK, &tgUser, configuration.PhoneEngineer, client, &ctx) {
							text := fmt.Sprintf("У вас нет прав на этот объект!\nВведите пультовый номер объекта!")
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
							msg.ReplyToMessageID = update.Message.MessageID
						} else {
							msg = tgbotapi.NewMessage(chatID, "Работа с объектом "+update.Message.Text)
							msg.ReplyToMessageID = update.Message.MessageID
							outMsg, _ := bot.Send(msg)
							pinMessage := tgbotapi.PinChatMessageConfig{
								ChatID:              chatID,
								MessageID:           outMsg.MessageID,
								DisableNotification: false,
							}
							_, _ = bot.Send(pinMessage)
							msg = createMenu(chatID, currentOperation[chatID])
						}
					} else if update.Message.Text != "" && currentOperation[chatID].currentRequest == "" {
						//Обработки ответов пользователя для работы с объектом
						msg = tgbotapi.NewMessage(chatID, "Работа с объектом "+update.Message.Text)
						msg.ReplyToMessageID = update.Message.MessageID
						msg = createMenu(chatID, currentOperation[chatID])
					}
				}
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Бот работает только в приватных чатах")
				msg.ReplyToMessageID = update.Message.MessageID
			}
		}

		if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {

			chatID := update.CallbackQuery.Message.Chat.ID

			if update.CallbackQuery.Data == "Finish" {
				text := fmt.Sprintf("Завершена работа с объектом %s", currentOperation[chatID].numberObject)
				msg = tgbotapi.NewMessage(chatID, text)
				msg.ReplyToMessageID = update.CallbackQuery.Message.MessageID
				_, _ = bot.Send(msg)
				unpinMessage := tgbotapi.UnpinAllChatMessagesConfig{
					ChatID: chatID,
				}
				_, _ = bot.Send(unpinMessage)

				currentOperation[chatID] = operation{}
				msg = tgbotapi.NewMessage(chatID, "Введите пультовый номер объекта!")
			} else if update.CallbackQuery.Data == "Back" {
				text := fmt.Sprintf("Работа с объектом %s", currentOperation[chatID].numberObject)
				if currentOperation[chatID].prevMenu == "" {
					msg = tgbotapi.NewMessage(chatID, text)
				} else {
					text += "\nПодменю MyAlarm"
					msg = tgbotapi.NewMessage(chatID, text)
				}
				currentOperation[chatID] = operation{
					numberObject:   currentOperation[chatID].numberObject,
					object:         currentOperation[chatID].object,
					customers:      currentOperation[chatID].customers,
					currentRequest: "",
					prevMenu:       currentOperation[chatID].prevMenu,
					checkPanicId:   currentOperation[chatID].checkPanicId,
					changedUserId:  currentOperation[chatID].changedUserId,
				}
				msg.ReplyToMessageID = update.CallbackQuery.Message.MessageID
				msg = createMenu(chatID, currentOperation[chatID])

			} else if update.CallbackQuery.Data == "GetCustomers" {
				currentOperation[chatID] = operation{
					numberObject:   currentOperation[chatID].numberObject,
					object:         currentOperation[chatID].object,
					customers:      currentOperation[chatID].customers,
					currentRequest: "GetCustomers",
					prevMenu:       "",
					checkPanicId:   "",
					changedUserId:  "",
				}

				text := ""
				for _, customer := range currentOperation[chatID].customers {
					text += fmt.Sprintf("№: %d\nФИО: %s\nТел.: %s\n\n", customer.UserNumber, customer.ObjCustName, customer.ObjCustPhone1)
				}
				msg = tgbotapi.NewMessage(chatID, text)
				msg.ReplyMarkup = addButtons(currentOperation[chatID].currentRequest, nil)
			} else if update.CallbackQuery.Data == "ChecksKTS" {
				currentOperation[chatID] = operation{
					numberObject:   currentOperation[chatID].numberObject,
					object:         currentOperation[chatID].object,
					customers:      currentOperation[chatID].customers,
					currentRequest: "ChecksKTS",
					prevMenu:       "",
					checkPanicId:   "",
					changedUserId:  "",
				}
				msg = checksKTSRequest(&currentOperation, chatID, confSDK, client, ctx)
				msg.ReplyToMessageID = update.Message.MessageID

			}
		}
		_, _ = bot.Send(msg)
	}
}
