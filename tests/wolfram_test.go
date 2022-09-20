package tests

import (
	"testing"

	"github.com/johnha/go-wolfram"
)

const WOLFRAM_APPID = "DEMO"

func TestGetQueryResult(t *testing.T) {
	c := &wolfram.Client{AppID: WOLFRAM_APPID}

	_, err := c.GetQueryResult("What is the price of gold?", nil)
	if err != nil {
		t.Failed()
		t.Log(err.Error())
	}
}

func TestGetSimpleQueryResult(t *testing.T) {
	c := &wolfram.Client{AppID: WOLFRAM_APPID}

	_, _, err := c.GetSimpleQuery("What is the price of gold?", nil)
	if err != nil {
		t.Failed()
		t.Log(err.Error())
	}
}

func TestGetFastQueryRecognizerResult(t *testing.T) {
	c := &wolfram.Client{AppID: WOLFRAM_APPID}

	_, err := c.GetFastQueryRecognizer("Gold price", wolfram.Default)
	if err != nil {
		t.Failed()
		t.Log(err.Error())
	}
}

func TestGetShortAnswerQueryResult(t *testing.T) {
	c := &wolfram.Client{AppID: WOLFRAM_APPID}

	_, err := c.GetShortAnswerQuery("Price of gold", wolfram.Metric, 0)
	if err != nil {
		t.Failed()
		t.Log(err.Error())
	}
}

func TestGetSpokenAnswerResult(t *testing.T) {
	c := &wolfram.Client{AppID: WOLFRAM_APPID}

	_, err := c.GetSpokentAnswerQuery("Price of gold", wolfram.Metric, 0)
	if err != nil {
		t.Failed()
		t.Log(err.Error())
	}
}
