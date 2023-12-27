package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	openai "github.com/sashabaranov/go-openai"
)

var conversationHistory []openai.ChatCompletionMessage

func updateConversationHistory(role, message string) {
	// Append new message to the conversation history
	conversationHistory = append(conversationHistory, openai.ChatCompletionMessage{
		Role:    role,
		Content: message,
	})
}

func main() {
	// Fetch environment configuration
	model := os.Getenv("MODEL")
	openaiKey := os.Getenv("OPENAI_API_KEY")
	baseUrl := os.Getenv("OPENAI_BASE_URL")

	// Configure OpenAI client
	config := openai.DefaultConfig(openaiKey)
	config.BaseURL = baseUrl

	var client *openai.Client
	if openaiKey != "" {
		client = openai.NewClientWithConfig(config)
	}

	// Initialize application
	app := tview.NewApplication()

	// Create a text view for input and output
	inputField := tview.NewInputField().
		SetLabel("Type Here: ").
		SetFieldBackgroundColor(tcell.ColorDefault).
		SetFieldTextColor(tcell.ColorGreen)

	chatHistory := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true)

	// Layout for the application
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(chatHistory, 0, 1, false).
		AddItem(inputField, 3, 1, true)

	// Set root and manage focus
	app.SetRoot(flex, true)

	// Function to handle streaming of messages
	handleStream := func(ctx context.Context, client *openai.Client, message string) {
		// Update conversation history with user's message
		updateConversationHistory(openai.ChatMessageRoleUser, message)

		req := openai.ChatCompletionRequest{
			Model:    model,
			Messages: conversationHistory, // Use the updated conversation history here
			Stream:   true,
		}

		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			chatHistory.SetText(chatHistory.GetText(true) + "\nStream error: " + err.Error())
			return
		}
		defer stream.Close()

		chatHistory.SetText(chatHistory.GetText(false) + "\nAgent: \n")
		var agentText string
		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				chatHistory.SetText(chatHistory.GetText(true) + "\nStream error: " + err.Error())
				return
			}

			// Append characters one by one for typing effect
			for _, char := range response.Choices[0].Delta.Content {
				currentText := chatHistory.GetText(false)
				chatHistory.SetText(currentText + string(char))
				app.Draw() // Force redraw of the application to update the view
				agentText = agentText + string(char)
			}
		}
		updateConversationHistory(openai.ChatMessageRoleSystem, agentText)
		chatHistory.SetText(chatHistory.GetText(false) + "\n")
	}

	// Input field handling
	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			ctx := context.Background() // Or manage context more granularly
			go handleStream(ctx, client, inputField.GetText())
			inputField.SetText("")
		}
	})

	// Graceful Shutdown with Ctrl+C
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		app.Stop()
	}()

	// Run the application
	if err := app.Run(); err != nil {
		panic(err)
	}
}
