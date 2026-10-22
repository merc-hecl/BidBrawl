package main

import (
	"encoding/json"

	"github.com/egoist/mygo"
)

// Use the loaded page's language and paper controls for desktop confirmations.
func confirmOnPage(window *mygo.Window, zh, en, acceptZh, acceptEn string) (bool, error) {
	copy, err := json.Marshal([]string{zh, en, acceptZh, acceptEn})
	if err != nil {
		return false, err
	}
	script, err := pages.ReadFile("web/prompt.js")
	if err != nil {
		return false, err
	}
	result, err := window.Page().Eval("return await (" + string(script) + ")(" + string(copy) + ")")
	if err == nil {
		accepted, _ := result.(bool)
		return accepted, nil
	}
	// A crashed renderer must not prevent the user from exiting the host.
	answer, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageQuestion,
		Title: "BidBrawl", Message: text(zh, en), Buttons: []string{text("取消", "Cancel"), text(acceptZh, acceptEn)}, CancelButton: 0, DefaultButton: 0})
	return err == nil && answer.Button == 1, err
}
