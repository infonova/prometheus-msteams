package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// CardCounter displays in the logs
var CardCounter int

// PrometheusAlertMessage is the request body that Prometheus sent via Generic Webhook
// The Documentation is in https://prometheus.io/docs/alerting/configuration/#webhook_config
type PrometheusAlertMessage struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// Alert construct is used by the PrometheusAlertMessage.Alerts
type Alert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
}

// TeamsMessageCard is for the Card Fields to send in Teams
// The Documentation is in https://docs.microsoft.com/en-us/outlook/actionable-messages/card-reference#card-fields
type TeamsMessageCard struct {
	Type       string                    `json:"@type"`
	Context    string                    `json:"@context"`
	ThemeColor string                    `json:"themeColor"`
	Summary    string                    `json:"summary"`
	Title      string                    `json:"title"`
	Text       string                    `json:"text,omitempty"`
	Sections   []TeamsMessageCardSection `json:"sections"`
}

// TeamsMessageCardSection is placed under TeamsMessageCard.Sections
// Each element of AlertWebHook.Alerts will the number of elements of TeamsMessageCard.Sections to create
type TeamsMessageCardSection struct {
	ActivityTitle string                         `json:"activityTitle"`
	Facts         []TeamsMessageCardSectionFacts `json:"facts"`
	Markdown      bool                           `json:"markdown"`
}

// TeamsMessageCardSectionFacts is placed under TeamsMessageCardSection.Facts
type TeamsMessageCardSectionFacts struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AlertManagerHandler handles incoming request to /alertmanager
func AlertManagerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Error: Only accepts POST requests.", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(r.Body)
	var p PrometheusAlertMessage
	err := decoder.Decode(&p)
	if err != nil {
		msg := fmt.Sprintf("Error: encoding message: %v", err)
		log.Println(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	// For Debugging, display the Request in JSON Format
	log.Println("Request received")
	json.NewEncoder(os.Stdout).Encode(p)
	// Create the Card
	c := new(TeamsMessageCard)
	c.CreateCard(p)
	// For Debugging, display the Request Body to send in JSON Format
	log.Println("Creating a card")
	json.NewEncoder(os.Stdout).Encode(c)
	err = c.SendCard()
	if err != nil {
		log.Println(err)
	}
}

// SendCard sends the JSON Encoded TeamsMessageCard
func (c *TeamsMessageCard) SendCard() error {
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(c)
	url := os.Getenv("TEAMS_INCOMING_WEBHOOK_URL")
	resp, err := http.Post(url, "application/json", b)
	if err != nil {
		log.Println(err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Error: %s", resp.Status)
	}
	CardCounter++
	log.Printf("Total Card sent since uptime: %d\n", CardCounter)
	return nil
}

// CreateCard creates the TeamsMessageCard based on values gathered from PrometheusAlertMessage
func (c *TeamsMessageCard) CreateCard(p PrometheusAlertMessage) error {
	const (
		messageType   = "MessageCard"
		context       = "http://schema.org/extensions"
		colorResolved = "2DC72D"
		colorFiring   = "8C1A1A"
		colorUnknown  = "CCCCCC"
	)
	c.Type = messageType
	c.Context = context
	switch p.Status {
	case "resolved":
		c.ThemeColor = colorResolved
	case "firing":
		c.ThemeColor = colorFiring
	default:
		c.ThemeColor = colorUnknown
	}
	c.Title = fmt.Sprintf("Prometheus Alert (%s)", p.Status)
	if value, notEmpty := p.CommonAnnotations["summary"]; notEmpty {
		c.Summary = value
	}
	useMarkdown := false
	if v := os.Getenv("MARKDOWN_ENABLED"); v == "yes" {
		useMarkdown = true
	}
	for _, alert := range p.Alerts {
		var s TeamsMessageCardSection
		s.ActivityTitle = fmt.Sprintf("[%s](%s)", alert.Annotations["description"], p.ExternalURL)
		s.Markdown = useMarkdown
		for key, val := range alert.Annotations {
			s.Facts = append(s.Facts, TeamsMessageCardSectionFacts{key, val})
		}
		for key, val := range alert.Labels {
			s.Facts = append(s.Facts, TeamsMessageCardSectionFacts{key, val})
		}
		c.Sections = append(c.Sections, s)
	}
	return nil
}
