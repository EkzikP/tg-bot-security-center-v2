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
		usersMyAlarm   []andromeda.UserMyAlarmResponse
		currentRequest string
		currentMenu    string
		checkPanicId   string
		changedUserId  string
	}

	menu struct {
		text         string
		callbackData string
	}
)

func newOperation() *operation {
	return &operation{}
}

func (o *operation) changeValue(fieldName string, value interface{}) {

	switch fieldName {
	case "numberObject":
		o.numberObject = value.(string)
	case "object":
		o.object = value.(andromeda.GetSitesResponse)
	case "customers":
		o.customers = value.([]andromeda.GetCustomerResponse)
	case "usersMyAlarm":
		o.usersMyAlarm = value.([]andromeda.UserMyAlarmResponse)
	case "currentRequest":
		o.currentRequest = value.(string)
	case "currentMenu":
		o.currentMenu = value.(string)
	case "checkPanicId":
		o.checkPanicId = value.(string)
	case "changedUserId":
		o.changedUserId = value.(string)
	}

}

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

	contactPhone, err := checkFormatPhone(update.Message.Contact.PhoneNumber)
	if err != nil {
		return false
	}

	if phone, ok := (*tgUser)[chatID]; !ok {
		err = addUser(chatID, contactPhone, tgUser)
		if err != nil {
			return false
		}
		return true
	} else if phone != contactPhone {
		err = addUser(chatID, contactPhone, tgUser)
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
func checkUserRights(object andromeda.GetSitesResponse, operation *operation, chatID int64, confSDK andromeda.Config, tgUser *map[string]string, phoneEngineer map[string]string, client *andromeda.Client, ctx *context.Context) bool {

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

	operation.changeValue("numberObject", strconv.Itoa(object.AccountNumber))
	operation.changeValue("object", object)
	operation.changeValue("customers", getCustomersResponse)
	operation.changeValue("currentMenu", "MainMenu")
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
func createMenu(chatId int64, operation *operation) tgbotapi.MessageConfig {

	if operation.currentMenu == "MyAlarmMenu" {
		msg := createMyAlarmMenu(chatId, operation)
		return msg
	}

	msg := createMainMenu(chatId, operation)
	return msg
}

// createMainMenu создает главное меню
func createMainMenu(chatID int64, operation *operation) tgbotapi.MessageConfig {
	mainMenu := []menu{
		{"Получить список ответственных лиц объекта", "GetCustomers"},
		{"Проверка КТС", "ChecksKTS"},
		{"Управление доступом в MyAlarm (в работе)", "MyAlarm"},
		{"Операции с карточкой объектов (не сделано)", "Object"},
		{"Управление разделами объекта (не сделано)", "Sections"},
		{"Управление шлейфами (не сделано)", "Shift"},
		{"Завершить работу с объектом", "Finish"},
	}

	operation.changeValue("currentMenu", "MainMenu")
	operation.changeValue("currentRequest", "")

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

// createMyAlarmMenu создает меню MyAlarm
func createMyAlarmMenu(chatID int64, operation *operation) tgbotapi.MessageConfig {
	mainMenu := []menu{
		{"Список пользователей MyAlarm объекта", "GetUsersMyAlarm"},
		{"Список объектов пользователя MyAlarm", "GetUserObjectMyAlarm"},
		{"Забрать доступ к MyAlarm", "PutDelUserMyAlarm"},
		{"Предоставить доступ к MyAlarm", "PutAddUserMyAlarm"},
		{"Модифицировать виртуальную КТС", "PutChangeVirtualKTS"},
		{"Назад", "Back"},
		{"Завершить работу с объектом", "Finish"},
	}

	keyboard := tgbotapi.InlineKeyboardMarkup{}
	for _, button := range mainMenu {
		var row []tgbotapi.InlineKeyboardButton
		btn := tgbotapi.NewInlineKeyboardButtonData(button.text, button.callbackData)
		row = append(row, btn)
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
	}
	text := fmt.Sprintf("Работа с объектом %s!\nПодменю MyAlarm.\nВыберите пункт меню:", operation.numberObject)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = &keyboard
	return msg
}

// addBtnBack добавляет кнопку назад к сообщениям
func addButtons(currentRequest string, enableResult bool) tgbotapi.InlineKeyboardMarkup {

	keyboard := tgbotapi.NewInlineKeyboardMarkup()

	if (currentRequest == "ChecksKTS" || currentRequest == "ResultCheckKTS") && enableResult {
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
func checksKTSRequest(operation *operation, chatID int64, confSDK andromeda.Config, client *andromeda.Client, ctx context.Context) tgbotapi.MessageConfig {

	if operation.currentRequest == "ChecksKTS" {
		PostCheckPanicRequest := andromeda.PostCheckPanicInput{
			SiteId: operation.object.Id,
			Config: confSDK,
		}
		PostCheckPanicResponse, err := client.PostCheckPanic(ctx, PostCheckPanicRequest)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "Не удалось получить данные")
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
			return msg
		}

		PostCheckPanic := map[string]string{
			"has alarm":                   "по объекту есть тревога, проверка КТС запрещена",
			"already runnig":              "по объекту уже выполняется проверка КТС",
			"success":                     "проверка КТС начата",
			"error":                       "при выполнении запроса произошла ошибка",
			"invalid checkInterval value": "для параметра checkInterval задано значение, выходящее за пределы допустимого диапазона",
		}

		if PostCheckPanicResponse.Description == "already runnig" {
			text := fmt.Sprintf("По объекту уже выполняется проверка КТС.\nДождитесь автоматического завершения проверки (макс. 3 мин.) или " +
				"отправьте тревогу КТС, для завершения ранее начатой проверки.\nИ повторите попытку снова.")
			msg := tgbotapi.NewMessage(chatID, text)
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
			return msg
		} else if PostCheckPanicResponse.Description != "success" {
			msg := tgbotapi.NewMessage(chatID, PostCheckPanic[PostCheckPanicResponse.Description])
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
			return msg
		}

		operation.changeValue("checkPanicId", PostCheckPanicResponse.CheckPanicId)

		text := fmt.Sprintf("%s\nВ течении 180 сек. нажмите кнпку КТС.\nИ нажмите кнопку \"Получить результат проверки КТС\"", PostCheckPanic[PostCheckPanicResponse.Description])
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyMarkup = addButtons(operation.currentRequest, true)
		return msg
	} else if operation.currentRequest == "ResultCheckKTS" {

		GetCheckPanicRequest := andromeda.GetCheckPanicInput{
			CheckPanicId: operation.checkPanicId,
			Config:       confSDK,
		}

		GetCheckPanicResponse, err := client.GetCheckPanic(ctx, GetCheckPanicRequest)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, err.Error())
			msg.ReplyMarkup = addButtons(operation.currentRequest, true)
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
		if GetCheckPanicResponse.Description == "in progress" {
			msg.ReplyMarkup = addButtons(operation.currentRequest, true)
		} else {
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
		}
		return msg
	}
	msg := tgbotapi.NewMessage(chatID, "Неизвестная команда")
	msg.ReplyMarkup = addButtons(operation.currentRequest, false)
	return msg
}

// getUsersMyAlarm получение данных о пользователях MyAlarm
func getUsersMyAlarm(ctx context.Context, client *andromeda.Client, confSDK andromeda.Config, operation *operation, chatID int64) tgbotapi.MessageConfig {

	usersMyAlarmRequest := andromeda.GetUsersMyAlarmInput{
		SiteId: operation.object.Id,
		Config: confSDK,
	}
	usersMyAlarmResponse, err := client.GetUsersMyAlarm(ctx, usersMyAlarmRequest)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "Не удалось получить данные")
		msg.ReplyMarkup = addButtons(operation.currentRequest, false)
		return msg
	}

	text := ""
	if len(usersMyAlarmResponse) == 0 {
		text = "Не найдено ни одного пользователя"
	}
	for _, user := range usersMyAlarmResponse {
		var kts string
		var role string
		if user.IsPanic {
			kts = "Да"
		} else {
			kts = "Нет"
		}

		if user.Role == "admin" {
			role = "Администратор"
		} else {
			role = "Пользователь"
		}
		userMyAlarmRequest := andromeda.GetCustomerInput{
			Id:     user.CustomerID,
			Config: confSDK,
		}

		userMyAlarmResponse, err := client.GetCustomer(ctx, userMyAlarmRequest)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "Не удалось получить данные")
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
			return msg
		}

		text += fmt.Sprintf("ФИО: %s\nТел.: %s\nРоль: %s\nКТС: %s\n\n", userMyAlarmResponse.ObjCustName, user.MyAlarmPhone, role, kts)
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = addButtons(operation.currentRequest, false)
	return msg
}

// haveMyAlarmRights проверяет права пользователя на систему MyAlarm и получает данные о пользователях системы MyAlarm
func haveMyAlarmRights(ctx context.Context, client *andromeda.Client, confSDK andromeda.Config, operation *operation, chatID int64, tgUser map[string]string, phoneEngineer map[string]string) bool {

	usersMyAlarmRequest := andromeda.GetUsersMyAlarmInput{
		SiteId: operation.object.Id,
		Config: confSDK,
	}
	usersMyAlarmResponse, err := client.GetUsersMyAlarm(ctx, usersMyAlarmRequest)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "Не удалось получить данные")
		msg.ReplyMarkup = addButtons(operation.currentRequest, false)
		return false
	}

	operation.changeValue("usersMyAlarm", usersMyAlarmResponse)

	phoneUser := tgUser[strconv.FormatInt(chatID, 10)]

	var validUser bool
	for _, user := range operation.usersMyAlarm {
		if user.MyAlarmPhone == phoneUser {
			validUser = true
			break
		}
	}

	if !validUser && !isEngineer(phoneUser, phoneEngineer) {
		return false
	}

	return true
}

// getUserObjectMyAlarm получает объекты пользователя MyAlarm
func getUserObjectMyAlarm(tgUser map[string]string, chatID int64, phoneEngineer map[string]string, operation *operation, update *tgbotapi.Update, ctx context.Context, client *andromeda.Client, confSDK andromeda.Config) tgbotapi.MessageConfig {

	var err error

	phone := tgUser[strconv.FormatInt(chatID, 10)]

	if isEngineer(phone, phoneEngineer) {
		if update.Message == nil {
			msg := tgbotapi.NewMessage(chatID, "Введите номер телефона пользователя в формате: +7xxxxxxxxxx")
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
			return msg
		} else {
			phone, err = checkFormatPhone(update.Message.Text)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, err.Error())
				msg.ReplyMarkup = addButtons(operation.currentRequest, false)
				return msg
			}
		}
	}

	userObjectMyAlarmRequest := andromeda.GetUserObjectMyAlarmInput{
		Phone:  phone,
		Config: confSDK,
	}

	userObjectMyAlarmResponse, err := client.GetUserObjectMyAlarm(ctx, userObjectMyAlarmRequest)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, err.Error())
		msg.ReplyMarkup = addButtons(operation.currentRequest, false)
		return msg
	}
	if len(userObjectMyAlarmResponse) == 0 {
		msg := tgbotapi.NewMessage(chatID, "У пользователя с номером "+phone+" нет объектов в приложении MyAlarm")
		msg.ReplyMarkup = addButtons(operation.currentRequest, false)
		return msg
	}

	text := ""
	for _, object := range userObjectMyAlarmResponse {
		var kts string
		var role string
		if object.IsPanic {
			kts = "Да"
		} else {
			kts = "Нет"
		}

		if object.Role == "admin" {
			role = "Администратор"
		} else {
			role = "Пользователь"
		}

		getSiteRequest := andromeda.GetSitesInput{
			Id:     object.ObjectGUID,
			Config: confSDK,
		}

		getSiteResponse, err := client.GetSites(ctx, getSiteRequest)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, err.Error())
			msg.ReplyMarkup = addButtons(operation.currentRequest, false)
			return msg
		}

		text += fmt.Sprintf("№ объекта: %d\nНаименование: %s\nАдрес: %s\nРоль: %s\nКТС: %s\n\n", getSiteResponse.AccountNumber, getSiteResponse.Name, getSiteResponse.Address, role, kts)
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = addButtons(operation.currentRequest, false)
	return msg
}

func checkFormatPhone(phone string) (string, error) {

	var userPhone string

	switch len(phone) {
	case 11:
		userPhone = "+7" + phone[1:]
	case 12:
		userPhone = phone
	case 10:
		userPhone = "+7" + phone
	default:
		return "", errors.New("Неверный формат номера телефона\nВведите номер телефона в формате: +7xxxxxxxxxx")
	}
	return userPhone, nil
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

	currentOperation := make(map[int64]*operation)

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
						currentOperation[chatID] = newOperation()
						msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Введите пультовый номер объекта!")
						msg.ReplyToMessageID = update.Message.MessageID
					}
				} else {
					//Проверка номера объекта и прав пользователя для работы с этим объектом
					if currentOperation[chatID].numberObject == "" {
						if !checkPhone(&update, &tgUser) {
							msg = requestPhone(chatID)
							msg.ReplyToMessageID = update.Message.MessageID
						} else if update.Message.Contact != nil {
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Введите пультовый номер объекта!")
							msg.ReplyToMessageID = update.Message.MessageID
						} else if message, ok := checkNumberObject(update.Message.Text); !ok {
							text := fmt.Sprintf("%s\nВведите пультовый номер объекта!", message)
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
							msg.ReplyToMessageID = update.Message.MessageID
						} else if object, err := findObject(update.Message.Text, confSDK, client, &ctx); err != nil {
							text := fmt.Sprintf("%s\nВведите пультовый номер объекта!", err)
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
							msg.ReplyToMessageID = update.Message.MessageID
						} else if !checkUserRights(object, currentOperation[chatID], chatID, confSDK, &tgUser, configuration.PhoneEngineer, client, &ctx) {
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
					} else if update.Message.Text != "" {
						if isEngineer(tgUser[strconv.FormatInt(chatID, 10)], configuration.PhoneEngineer) &&
							currentOperation[chatID].currentRequest == "GetUserObjectMyAlarm" {
							msg = getUserObjectMyAlarm(tgUser, chatID, configuration.PhoneEngineer, currentOperation[chatID], &update, ctx, client, confSDK)
							msg.ReplyToMessageID = update.Message.MessageID
						} else {
							//Обработки ответов пользователя для работы с объектом
							msg = tgbotapi.NewMessage(chatID, "Работа с объектом "+update.Message.Text)
							msg.ReplyToMessageID = update.Message.MessageID
							currentOperation[chatID].changeValue("currentRequest", "")
							currentOperation[chatID].changeValue("checkPanicId", "")
							currentOperation[chatID].changeValue("changedUserId", "")
							msg = createMenu(chatID, currentOperation[chatID])
						}
					}
				}
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Бот работает только в приватных чатах")
				msg.ReplyToMessageID = update.Message.MessageID
			}
		}

		if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {

			chatID := update.CallbackQuery.Message.Chat.ID

			switch update.CallbackQuery.Data {
			case "Finish":
				text := fmt.Sprintf("Завершена работа с объектом %s", currentOperation[chatID].numberObject)
				msg = tgbotapi.NewMessage(chatID, text)
				_, _ = bot.Send(msg)
				unpinMessage := tgbotapi.UnpinAllChatMessagesConfig{
					ChatID: chatID,
				}
				_, _ = bot.Send(unpinMessage)

				currentOperation[chatID] = newOperation()
				msg = tgbotapi.NewMessage(chatID, "Введите пультовый номер объекта!")
			case "Back":
				text := fmt.Sprintf("Работа с объектом %s", currentOperation[chatID].numberObject)
				if currentOperation[chatID].currentMenu == "MyAlarmMenu" && currentOperation[chatID].currentRequest == "MyAlarm" {
					text += "\nПодменю MyAlarm"
					msg = tgbotapi.NewMessage(chatID, text)
					currentOperation[chatID].changeValue("currentRequest", "")
					currentOperation[chatID].changeValue("currentMenu", "MainMenu")
				} else if currentOperation[chatID].currentMenu == "MyAlarmMenu" {
					msg = tgbotapi.NewMessage(chatID, text)
					text += "\nПодменю MyAlarm"
					msg = tgbotapi.NewMessage(chatID, text)
					currentOperation[chatID].changeValue("currentRequest", "MyAlarm")
				} else {
					msg = tgbotapi.NewMessage(chatID, text)
					currentOperation[chatID].changeValue("currentRequest", "")
				}
				msg.ReplyToMessageID = update.CallbackQuery.Message.MessageID
				msg = createMenu(chatID, currentOperation[chatID])
			case "GetCustomers":
				currentOperation[chatID].changeValue("currentRequest", update.CallbackQuery.Data)

				text := ""
				for _, customer := range currentOperation[chatID].customers {
					text += fmt.Sprintf("№: %d\nФИО: %s\nТел.: %s\n\n", customer.UserNumber, customer.ObjCustName, customer.ObjCustPhone1)
				}
				msg = tgbotapi.NewMessage(chatID, text)
				msg.ReplyMarkup = addButtons(currentOperation[chatID].currentRequest, false)
			case "ChecksKTS", "ResultCheckKTS":
				currentOperation[chatID].changeValue("currentRequest", update.CallbackQuery.Data)
				msg = checksKTSRequest(currentOperation[chatID], chatID, confSDK, client, ctx)
				msg.ReplyToMessageID = update.CallbackQuery.Message.MessageID
			case "MyAlarm":
				if haveMyAlarmRights(ctx, client, confSDK, currentOperation[chatID], chatID, tgUser, configuration.PhoneEngineer) {
					currentOperation[chatID].changeValue("currentRequest", update.CallbackQuery.Data)
					currentOperation[chatID].changeValue("currentMenu", "MyAlarmMenu")
					msg = createMenu(chatID, currentOperation[chatID])
				} else {
					msg = tgbotapi.NewMessage(chatID, "У вас нет прав на работу с системой MyAlarm")
					_, _ = bot.Send(msg)
					msg = createMenu(chatID, currentOperation[chatID])
				}
			case "GetUsersMyAlarm":
				currentOperation[chatID].changeValue("currentRequest", update.CallbackQuery.Data)
				msg = getUsersMyAlarm(ctx, client, confSDK, currentOperation[chatID], chatID)
				msg.ReplyToMessageID = update.CallbackQuery.Message.MessageID
			case "GetUserObjectMyAlarm":
				currentOperation[chatID].changeValue("currentRequest", update.CallbackQuery.Data)
				msg = getUserObjectMyAlarm(tgUser, chatID, configuration.PhoneEngineer, currentOperation[chatID], &update, ctx, client, confSDK)
				msg.ReplyToMessageID = update.CallbackQuery.Message.MessageID
			}
		}
		_, _ = bot.Send(msg)
	}
}
