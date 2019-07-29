package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
)

func main() {
	if os.Getenv("IS_HEROKU") != "TRUE" {
		loadEnvironmentalVariables()
	}

	//set up telegram info
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_TOKEN"))
	errCheck(err, "Failed to start telegram bot")
	log.Printf("Authorized on account %s", bot.Self.UserName)
	chatIDs, err := parseChatIDList((os.Getenv("CHAT_ID")))
	errCheck(err, "Failed to fetch chat IDs")

	client := &http.Client{}
	tgclient := AlertService{Bot: bot, ReceiverIDs: chatIDs}

	//for heroku
	go func() {
		http.ListenAndServe(":"+os.Getenv("PORT"),
			http.HandlerFunc(http.NotFound))
	}()

	for {
		//fetching session ID cookie
		log.Println("Logging in")
		logIn(os.Getenv("LOGIN_ID"), os.Getenv("PASSWORD"), client)

		//fetching the booking page (client now has cookie stored inside a jar)
		log.Println("Fetching booking page")
		rawPage := slotPage(client)

		log.Println("Parsing booking page")
		slots := extractDates(rawPage)
		valids := validSlots(slots)

		for _, validSlot := range valids { //for all the slots which meet the rule (i.e. within 10 days of now)
			tgclient.MessageAll("Slot available on " + validSlot.Date.Format("_2 Jan 2006 (Mon)") + " " + os.Getenv("SESSION_"+validSlot.SessionNumber))
		}
		if len(valids) != 0 {
			tgclient.MessageAll("Finished getting slots")
		}

		r := rand.Intn(300) + 120
		time.Sleep(time.Duration(r) * time.Second)
	}

}

func parseChatIDList(list string) ([]int64, error) {
	chatIDStrings := strings.Split(list, ",")
	chatIDs := make([]int64, len(chatIDStrings))
	for i, chatIDString := range chatIDStrings {
		chatID, err := strconv.ParseInt(strings.TrimSpace(chatIDString), 10, 64)
		chatIDs[i] = chatID
		if err != nil {
			return nil, err
		}
	}
	return chatIDs, nil
}

func alert(msg string, bot *tgbotapi.BotAPI, chatID int64) {
	telegramMsg := tgbotapi.NewMessage(chatID, msg)
	bot.Send(telegramMsg)
	log.Println("Sent message to " + strconv.FormatInt(chatID, 10) + ": " + msg)
}

// AlertService is a service for alerting many telegram users
type AlertService struct {
	Bot         *tgbotapi.BotAPI
	ReceiverIDs []int64
}

// Sends a message to all chats in the alert service
func (as *AlertService) MessageAll(msg string) {
	for _, chatID := range as.ReceiverIDs {
		alert(msg, as.Bot, chatID)
	}
}

func loadEnvironmentalVariables() {
	err := godotenv.Load()
	if err != nil {
		log.Print("Error loading environmental variables: ")
		log.Fatal(err.Error())
	}
}

// Returns which of the slots the user should be alerted about (ie valid slots)
func validSlots(slots []DrivingSlot) []DrivingSlot {
	valid := make([]DrivingSlot, 0)

	for _, slot := range slots {
		if slot.Date.Sub(time.Now()) < 10*(24*time.Hour) { //if slot is within 10 days of now
			valid = append(valid, slot)
		}
	}

	return valid
}

type myjar struct {
	jar map[string][]*http.Cookie
}

func (p *myjar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	fmt.Printf("The URL is : %s\n", u.String())
	fmt.Printf("The cookie being set is : %s\n", cookies)
	p.jar[u.Host] = cookies
}

func (p *myjar) Cookies(u *url.URL) []*http.Cookie {
	fmt.Printf("The URL is : %s\n", u.String())
	fmt.Printf("Cookie being returned is : %s\n", p.jar[u.Host])
	return p.jar[u.Host]
}

// logIn logs into the CDC website, starting a session.
// Returns the cookie storing the session data
func logIn(learnerID string, pwd string, client *http.Client) {
	loginForm := url.Values{}
	loginForm.Add("LearnerID", learnerID)
	loginForm.Add("Pswd", pwd)
	req, err := http.NewRequest("POST", "https://www.cdc.com.sg/NewPortal/", strings.NewReader(loginForm.Encode()))
	errCheck(err, "Error making log in request")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/ /*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://www.cdc.com.sg/")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:68.0) Gecko/20100101 Firefox/68.0")
	jar := &myjar{}
	jar.jar = make(map[string][]*http.Cookie)
	client.Jar = jar
	_, err = client.Do(req)

	errCheck(err, "Error logging in and getting session cookie")
}

// Returns the page containing all the slot information
func slotPage(client *http.Client) string {
	reqBody := strings.NewReader("ctl00$ContentPlaceHolder1$ScriptManager1=ctl00$ContentPlaceHolder1$UpdatePanel1|ctl00$ContentPlaceHolder1$ddlCourse&ctl00_Menu1_TreeView1_ExpandState=eennnnnnennennenneunnnnnnnnnnnnnenen&ctl00_Menu1_TreeView1_SelectedNode=&__EVENTTARGET=ctl00%24ContentPlaceHolder1%24ddlCourse&__EVENTARGUMENT=&ctl00_Menu1_TreeView1_PopulateLog=&__LASTFOCUS=&__VIEWSTATE=%2FwEPDwULLTE2OTQyNTIyODkPZBYCZg9kFgICAw9kFgYCAQ9kFgICCQ88KwAJAgAPFgoeC18hRGF0YUJvdW5kZx4JTGFzdEluZGV4AiQeDU5ldmVyRXhwYW5kZWRkHgxEYXRhU291cmNlSUQFC1htbERhdGFtZW51HgxTZWxlY3RlZE5vZGVkZAgUKwACBQMwOjAUKwACFgweBFRleHQFBEhvbWUeBVZhbHVlBQRIb21lHgtOYXZpZ2F0ZVVybAUYfi9Cb29raW5nL0Rhc2hib2FyZC5hc3B4HghEYXRhUGF0aAUQLypbcG9zaXRpb24oKT0xXR4JRGF0YUJvdW5kZx4IRXhwYW5kZWRnFCsACAUbMDowLDA6MSwwOjIsMDozLDA6NCwwOjUsMDo2FCsAAhYMHwUFB0Jvb2tpbmcfBgUHQm9va2luZx4MU2VsZWN0QWN0aW9uCyouU3lzdGVtLldlYi5VSS5XZWJDb250cm9scy5UcmVlTm9kZVNlbGVjdEFjdGlvbgMfCAUgLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9MV0fCWcfCmcUKwAHBRcwOjAsMDoxLDA6MiwwOjMsMDo0LDA6NRQrAAIWCh8FBRhlLVRyaWFsIFRlc3QgKENsYXNzcm9vbSkfBgUYZS1UcmlhbCBUZXN0IChDbGFzc3Jvb20pHwcFHH4vQm9va2luZy9Cb29raW5nRVRyaWFsLmFzcHgfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTFdHwlnZBQrAAIWCh8FBRNJbnRlcm5hbCBFdmFsdWF0aW9uHwYFE0ludGVybmFsIEV2YWx1YXRpb24fBwUYfi9Cb29raW5nL0Jvb2tpbmdJRS5hc3B4HwgFMC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT0yXR8JZ2QUKwACFgofBQUNVGhlb3J5IExlc3Nvbh8GBQ1UaGVvcnkgTGVzc29uHwcFGH4vQm9va2luZy9Cb29raW5nVEwuYXNweB8IBTAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9M10fCWdkFCsAAhYKHwUFC1RoZW9yeSBUZXN0HwYFC1RoZW9yeSBUZXN0HwcFGH4vQm9va2luZy9Cb29raW5nVFQuYXNweB8IBTAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9NF0fCWdkFCsAAhYKHwUFEFByYWN0aWNhbCBMZXNzb24fBgUQUHJhY3RpY2FsIExlc3Nvbh8HBRh%2BL0Jvb2tpbmcvQm9va2luZ1BMLmFzcHgfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTVdHwlnZBQrAAIWCh8FBQ5QcmFjdGljYWwgVGVzdB8GBQ5QcmFjdGljYWwgVGVzdB8HBRh%2BL0Jvb2tpbmcvQm9va2luZ1BULmFzcHgfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTZdHwlnZBQrAAIWDB8FBQ9PbmxpbmUgU2VydmljZXMfBgUPT25saW5lIFNlcnZpY2VzHwsLKwQDHwgFIC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTJdHwlnHwpnFCsAAwUHMDowLDA6MRQrAAIWCh8FBRNlLUxlYXJuaW5nIChPbmxpbmUpHwYFE2UtTGVhcm5pbmcgKE9ubGluZSkfBwUbfi9Cb29raW5nL1NjaEVMZWFybmluZy5hc3B4HwgFMC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTJdLypbcG9zaXRpb24oKT0xXR8JZ2QUKwACFgofBQUVZS1UcmlhbCBUZXN0IChPbmxpbmUpHwYFFWUtVHJpYWwgVGVzdCAoT25saW5lKR8HBRt%2BL0Jvb2tpbmcvT25saW5lRVRyaWFsLmFzcHgfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9Ml0vKltwb3NpdGlvbigpPTJdHwlnZBQrAAIWDB8FBR1DYW5jZWxsYXRpb24vUmUtUHJpbnQgUmVjZWlwdB8GBR1DYW5jZWxsYXRpb24vUmUtUHJpbnQgUmVjZWlwdB8LCysEAx8IBSAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT0zXR8JZx8KZxQrAAMFBzA6MCwwOjEUKwACFgofBQUMQ2FuY2VsbGF0aW9uHwYFDENhbmNlbGxhdGlvbh8HBRx%2BL0Jvb2tpbmcvQm9va2luZ0NhbmNlbC5hc3B4HwgFMC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTNdLypbcG9zaXRpb24oKT0xXR8JZ2QUKwACFgofBQUQUmUtUHJpbnQgUmVjZWlwdB8GBRBSZS1QcmludCBSZWNlaXB0HwcFGn4vQm9va2luZy9QcmludFJlcG9ydC5hc3B4HwgFMC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTNdLypbcG9zaXRpb24oKT0yXR8JZ2QUKwACFgwfBQUVVHJhbnNhY3Rpb24gU3RhdGVtZW50HwYFFVRyYW5zYWN0aW9uIFN0YXRlbWVudB8LCysEAx8IBSAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT00XR8JZx8KZxQrAAMFBzA6MCwwOjEUKwACFgofBQUPQm9va2luZyBTdW1tYXJ5HwYFD0Jvb2tpbmcgU3VtbWFyeR8HBR9%2BL0Jvb2tpbmcvU3RhdGVtZW50Qm9va2luZy5hc3B4HwgFMC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTRdLypbcG9zaXRpb24oKT0xXR8JZ2QUKwACFgofBQUUU3RhdGVtZW50IG9mIEFjY291bnQfBgUUU3RhdGVtZW50IG9mIEFjY291bnQfBwUffi9Cb29raW5nL1N0YXRlbWVudEFjY291bnQuYXNweB8IBTAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT00XS8qW3Bvc2l0aW9uKCk9Ml0fCWdkFCsAAhYMHwUFEEN1c3RvbWVyIFNlcnZpY2UfBgUQQ3VzdG9tZXIgU2VydmljZR8LCysEAx8IBSAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XR8JZx8KZxQrAAYFEzA6MCwwOjEsMDoyLDA6MywwOjQUKwACFgofBQUSQ2FyIEFsbG9jYXRpb24gTWFwHwYFEkNhciBBbGxvY2F0aW9uIE1hcB8LCysEAx8IBTAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9MV0fCWcUKwAKBSMwOjAsMDoxLDA6MiwwOjMsMDo0LDA6NSwwOjYsMDo3LDA6OBQrAAIWCh8FBQNVYmkfBgUDVWJpHwcFHn4vQ2FyQWxsb2NhdGlvbk1hcC9VYmlNYXAuYXNweB8IBUAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTFdHwlnZBQrAAIWCh8FBQhCdWFuZ2tvax8GBQhCdWFuZ2tvax8HBSN%2BL0NhckFsbG9jYXRpb25NYXAvQnVhbmdrb2tNYXAuYXNweB8IBUAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTJdHwlnZBQrAAIWCh8FBQhGZXJudmFsZR8GBQhGZXJudmFsZR8HBSB%2BL0NhckFsbG9jYXRpb25NYXAvRmVybnZhbGUuYXNweB8IBUAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTNdHwlnZBQrAAIWCh8FBQVLb3Zhbh8GBQVLb3Zhbh8HBSB%2BL0NhckFsbG9jYXRpb25NYXAvS292YW5NYXAuYXNweB8IBUAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTRdHwlnZBQrAAIWCh8FBQxQb3RvbmcgUGFzaXIfBgUMUG90b25nIFBhc2lyHwcFI34vQ2FyQWxsb2NhdGlvbk1hcC9Qb3RvbmdQYXNpci5hc3B4HwgFQC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTVdLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9NV0fCWdkFCsAAhYKHwUFB1B1bmdnb2wfBgUHUHVuZ2dvbB8HBSJ%2BL0NhckFsbG9jYXRpb25NYXAvUHVuZ2dvbE1hcC5hc3B4HwgFQC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTVdLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9Nl0fCWdkFCsAAhYKHwUFCFNlbmdrYW5nHwYFCFNlbmdrYW5nHwcFI34vQ2FyQWxsb2NhdGlvbk1hcC9TZW5na2FuZ01hcC5hc3B4HwgFQC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTVdLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9N10fCWdkFCsAAhYKHwUFCVNlcmFuZ29vbh8GBQlTZXJhbmdvb24fBwUkfi9DYXJBbGxvY2F0aW9uTWFwL1NlcmFuZ29vbk1hcC5hc3B4HwgFQC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTVdLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9OF0fCWdkFCsAAhYKHwUFEFRhbXBpbmVzIENlbnRyYWwfBgUQVGFtcGluZXMgQ2VudHJhbB8HBSp%2BL0NhckFsbG9jYXRpb25NYXAvVGFtcGluZXNDZW50cmFsTWFwLmFzcHgfCAVALypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9NV0vKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT05XR8JZ2QUKwACFgofBQUSVG9wLVVwIFN0b3JlIFZhbHVlHwYFElRvcC1VcCBTdG9yZSBWYWx1ZR8HBSB%2BL0Jvb2tpbmcvQ1NUb3BVcFN0b3JlVmFsdWUuYXNweB8IBTAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9Ml0fCWdkFCsAAhYKHwUFEFJlbmV3IE1lbWJlcnNoaXAfBgUQUmVuZXcgTWVtYmVyc2hpcB8HBSB%2BL0Jvb2tpbmcvQ1NSZW5ld01lbWJlcnNoaXAuYXNweB8IBTAvKltwb3NpdGlvbigpPTFdLypbcG9zaXRpb24oKT01XS8qW3Bvc2l0aW9uKCk9M10fCWdkFCsAAhYKHwUFF1VwZGF0ZSBQZXJzb25hbCBEZXRhaWxzHwYFF1VwZGF0ZSBQZXJzb25hbCBEZXRhaWxzHwcFH34vQm9va2luZy9DU1VwZGF0ZVBlcnNvbmFsLmFzcHgfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9NV0vKltwb3NpdGlvbigpPTRdHwlnZBQrAAIWCh8FBQ9DaGFuZ2UgUGFzc3dvcmQfBgUPQ2hhbmdlIFBhc3N3b3JkHwcFH34vQm9va2luZy9DU0NoYW5nZVBhc3N3b3JkLmFzcHgfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9NV0vKltwb3NpdGlvbigpPTVdHwlnZBQrAAIWDB8FBRNFeGl0IEJvb2tpbmcgUG9ydGFsHwYFE0V4aXQgQm9va2luZyBQb3J0YWwfCwsrBAMfCAUgLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9Nl0fCWcfCmcUKwACBQMwOjAUKwACFgofBQUGTG9nb3V0HwYFBkxvZ291dB8HBR4uLi9sb2dPdXQuYXNweD9QYWdlTmFtZT1Mb2dvdXQfCAUwLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9Nl0vKltwb3NpdGlvbigpPTFdHwlnZBQrAAIWDB8FBQRIZWxwHwYFBEhlbHAfCwsrBAMfCAUgLypbcG9zaXRpb24oKT0xXS8qW3Bvc2l0aW9uKCk9N10fCWcfCmcUKwACBQMwOjAUKwACFgofBQUXVHVybiBPZmYgUG9wLXVwIEJsb2NrZXIfBgUXVHVybiBPZmYgUG9wLXVwIEJsb2NrZXIfBwUhfi9IZWxwL1BvcHVwQmxvY2tlckFzc2lzdGFudC5hc3B4HwgFMC8qW3Bvc2l0aW9uKCk9MV0vKltwb3NpdGlvbigpPTddLypbcG9zaXRpb24oKT0xXR8JZ2RkAgMPZBYQAgkPDxYCHwUFEEdvb2QgYWZ0ZXJub29uLCBkZAILDw8WAh8FBQ1MT08gV0VJIEpVQU4gZGQCEQ8PFgIfBQUIM0EwMjk2MDZkZAIVDw8WAh8FBSJDbGFzcyAzQSAoQXV0bykgUHJhY3RpY2FsICYgVGhlb3J5ZGQCGQ8PFgIfBQUJUzk5MTY0MjdBZGQCHQ8PFgIfBQUHJDI3Ni40MGRkAiEPDxYCHwUFCzE4LUp1bC0yMDIwZGQCJQ8PFgIfBQUGJDEwLjAwZGQCBQ9kFgoCCQ9kFgJmD2QWCAIDDxAPFgYeDURhdGFUZXh0RmllbGQFDVJlc0FzbWJseURlc2MeDkRhdGFWYWx1ZUZpZWxkBQtSZXNBc21CbHlJRB8AZ2QQFQIPLVBsZWFzZSBTZWxlY3QtMkNsYXNzIDNBIChBdXRvKSBQcmFjdGljYWwgJiBUaGVvcnkgICAgICAgICAgICAgICAgFQIAFEFVVE9DQVItQzNBICAgICAgICAgFCsDAmdnFgFmZAINDxBkZBYAZAIRD2QWAmYPZBYCAgEPZBYCAgMPEGRkFgECAmQCEw9kFgJmD2QWAgIBD2QWAgIDDxBkZBYAZAILD2QWAmYPZBYCAgUPDxYGHwUFHllvdSBoYXZlIDAgdW5jb25maXJtZWQgc2Vzc2lvbh4HVG9vbFRpcGUeB0VuYWJsZWRoZGQCGw9kFgJmD2QWAgIBDzwrAA0AZAIdD2QWAmYPZBYMAgEPDxYCHghJbWFnZVVybAUbfi9pbWFnZXMvQ2xhc3MzL2ltYWdlczEuZ2lmZGQCAw8PFgIfBQUJQXZhaWxhYmxlZGQCBQ8PFgIfEAUbfi9pbWFnZXMvQ2xhc3MzL2ltYWdlczIuZ2lmZGQCBw8PFgIfBQUeWW91IGhhdmUgcmVzZXJ2ZWQgdGhpcyBzZXNzaW9uZGQCCQ8PFgIfEAUbfi9pbWFnZXMvQ2xhc3MzL2ltYWdlczMuZ2lmZGQCCw8PFgIfBQUfWW91IGhhdmUgY29uZmlybWVkIHRoaXMgc2Vzc2lvbmRkAiEPZBYCZg9kFgICAQ8PFgIfD2hkZBgCBR5fX0NvbnRyb2xzUmVxdWlyZVBvc3RCYWNrS2V5X18WAQUVY3RsMDAkTWVudTEkVHJlZVZpZXcxBSRjdGwwMCRDb250ZW50UGxhY2VIb2xkZXIxJGd2TGF0ZXN0YXYPZ2RX95NR8Crjn947M9QWTdmqwcXjwg%3D%3D&__VIEWSTATEGENERATOR=01B9CB21&__PREVIOUSPAGE=FO0HqDtJ0ijl0L_h5XfSiYMUHiEDNdtlI3-b8XVoykYIBWrIaLSk2l8aQFgFDM7O6mJE9yXFoXlo6A5dbe261cjM8E-BqISG2r2c57HdUZTXPaIU0&__EVENTVALIDATION=%2FwEWEgK53MmvDgKPvfGzDQL28YKGAQLDxJ6eCgLDxJ6eCgLm8oXzAQLLg%2FWQCwLo3uSOAgKAgfWhBwKAgfmhBwKAgf2hBwLjlcPCBALD7PnoCQKXh7WvBAKb4t3aCwKMxLTvCAL6uakZAveoidUEsqd1ubnCwBB9bh4A%2BBlLGKUeFpo%3D&ctl00$ContentPlaceHolder1$ddlCourse=AUTOCAR-C3A%20%20%20%20%20%20%20%20%20&ctl00$ContentPlaceHolder1$hdReserveCnt=0&ctl00$ContentPlaceHolder1$hdTicketEndDate=&ctl00$ContentPlaceHolder1$hdM1=0&ctl00$ContentPlaceHolder1$hdM2=0&ctl00$ContentPlaceHolder1$hdM3=0&ctl00$ContentPlaceHolder1$hdCacheBackNav=5&ctl00$ContentPlaceHolder1$hdCacheForNav=5&ctl00$ContentPlaceHolder1$hdType=Class3&ctl00$ContentPlaceHolder1$hdDate=&ctl00$ContentPlaceHolder1$hdSession=&ctl00$ContentPlaceHolder1$hdDays=&")
	req, err := http.NewRequest("POST", "https://www.cdc.com.sg/NewPortal/Booking/BookingPL.aspx", reqBody)
	errCheck(err, "Error creating request for driving slots")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://www.cdc.com.sg/NewPortal/Booking/BookingPL.aspx")
	req.Header.Set("X-MicrosoftAjax", "Delta=true")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:68.0) Gecko/20100101 Firefox/68.0")

	resp, err := client.Do(req)
	errCheck(err, "Error getting driving slots")
	bytes, err := ioutil.ReadAll(resp.Body)
	errCheck(err, "Error reading request body")

	return string(bytes)
}

// DrivingSlot represents a CDC slot to go for driving lessons
type DrivingSlot struct {
	Date          time.Time
	SessionNumber string
}

// Given the output of the slot page, finds the
func extractDates(slotPage string) []DrivingSlot {
	daySections := strings.Split(slotPage, "</tr><tr>")[1:]
	slots := make([]DrivingSlot, 0)
	/* What a day section looks like
		<td>17/Sep/2019</td><td align="center">TUE</td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession1" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession1" src="../Images/Class3/Images0.gif" style="border-width:0px;" />
	                                                    </td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession2" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession2" src="../Images/Class3/Images1.gif" style="border-width:0px;" />
	                                                    </td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession3" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession3" src="../Images/Class3/Images0.gif" style="border-width:0px;" />
	                                                    </td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession4" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession4" src="../Images/Class3/Images0.gif" style="border-width:0px;" />
	                                                    </td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession5" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession5" src="../Images/Class3/Images3.gif" style="border-width:0px;" />
	                                                    </td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession6" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession6" src="../Images/Class3/Images0.gif" style="border-width:0px;" />
	                                                    </td><td align="center">
	                                                        <input type="image" name="ctl00$ContentPlaceHolder1$gvLatestav$ctl02$btnSession7" id="ctl00_ContentPlaceHolder1_gvLatestav_ctl02_btnSession7" src="../Images/Class3/Images0.gif" style="border-width:0px;" />
														</td>
	*/
	for _, daySection := range daySections {
		dateString := strings.Split(strings.Split(daySection, "</td>")[0], "<td>")[1]
		date, err := time.Parse("2/Jan/2006", dateString)
		errCheck(err, "Error parsing date of slot")
		rawSlots := strings.Split(daySection, `</td><td align="center">`)[2:] //exclude the first two to get to the actual cells in the table
		for _, rawSlot := range rawSlots {
			if openSlot(rawSlot) {
				sessionNum := strings.Split(strings.Split(rawSlot, "btnSession")[1], `"`)[0]
				log.Println(date, sessionNum)
				slots = append(slots, DrivingSlot{
					Date:          date,
					SessionNumber: sessionNum,
				})
			}
		}
	}
	return slots
}

// Returns true if a given raw slot is open
func openSlot(rawSlot string) bool {
	return strings.Contains(rawSlot, "Images1.gif")
}

func errCheck(err error, msg string) {
	if err != nil {
		log.Fatal(msg + ": " + err.Error())
	}
}
