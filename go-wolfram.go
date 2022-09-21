package wolfram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
	"github.com/valyala/fasttemplate"

	jsonIter "github.com/json-iterator/go"
)

var jsonLib = jsonIter.ConfigCompatibleWithStandardLibrary

/*
	See the following for detail on the API:

	https://products.wolframalpha.com/api/documentation
*/

// Client requires an App ID, which you can sign up for at https://developer.wolframalpha.com/
type Client struct {
	AppID string
}

type Query struct {
	Result QueryResult `json:"queryresult"`
}

//The QueryResult is what you get back after a request
type QueryResult struct {
	Query string

	//The pods are what hold the majority of the information
	Pods []Pod `json:"pods"`

	//Warnings hold information about for example spelling errors
	Warnings Warnings `json:"warnings"`

	//Assumptions show info if some assumption was made while parsing the query
	Assumptions Assumptions `json:"assumptions"`

	// Each Source contains a link to a web page with the source information.
	//  johnha: removed this from interpretation.  This is a single object when only 1, otherwise an array.   We need 1 or
	//	the other to unmarshall so will ignore.
	// Sources []Source `json:"sources"`

	//Generalizes the query to display more information
	Generalizations []Generalization `json:"generalization"`

	//true or false depending on whether the input could be successfully
	//understood. If false there will be no <pod> subelements
	Success bool `json:"success"`

	//true or false depending on whether a serious processing error occurred,
	//such as a missing required parameter. If true there will be no pod
	//content, just an <error> sub-element.
	Error bool `json:"error"`

	//The number of pod elements
	NumPods int `json:"numpods"`

	//Categories and types of data represented in the results (comma separated list)
	DataTypes string `json:"datatypes"`

	//The number of pods that are missing because they timed out (see the
	//scantimeout query parameter).
	TimedOut string `json:"timedout"`

	//The wall-clock time in seconds required to generate the output.
	Timing float64 `json:"timing"`

	//The time in seconds required by the parsing phase.
	ParseTiming float64 `json:"parsetiming"`

	//Whether the parsing stage timed out (try a longer parsetimeout parameter
	//if true)
	ParseTimedOut bool `json:"parsetimedout"`

	//A URL to use to recalculate the query and get more pods.
	ReCalculate string `json:"recalculate"`

	//These elements are not documented currently
	ID      string `json:"id"`
	Host    string `json:"host"`
	Server  string `json:"server"`
	Related string `json:"related"`

	//The version specification of the API on the server that produced this result.
	Version string `json:"version"`
}

type Generalization struct {
	Topic       string `json:"topic"`
	Description string `json:"desc"`
	URL         string `json:"url"`
}

type Warnings struct {
	//How many warnings were issued
	Count int `json:"count"`

	//Suggestions for spelling corrections
	Spellchecks []Spellcheck `json:"spellcheck"`

	//"If you enter a query with mismatched delimiters like "sin(x", Wolfram|Alpha attempts to fix the problem and reports
	//this as a warning."
	Delimiters []Delimiters `json:"delimiters"`

	//"[The API] will translate some queries from non-English languages into English. In some cases when it does
	//this, you will get a <translation> element in the API result."
	Translations []Translation `json:"translation"`

	//"[The API] can automatically try to reinterpret a query that it does not understand but that seems close to one
	//that it can."
	ReInterpretations []ReInterpretation `json:"reinterpret"`
}

type Spellcheck struct {
	Word       string `json:"word"`
	Suggestion string `json:"suggestion"`
	Text       string `json:"text"`
}

type Delimiters struct {
	Text string `json:"text"`
}

type Translation struct {
	Phrase      string `json:"phrase"`
	Translation string `json:"trans"`
	Language    string `json:"lang"`
	Text        string `json:"text"`
}

type ReInterpretation struct {
	Alternatives []Alternative `json:"alternative"`
	Text         string        `json:"text"`
	New          string        `json:"new"`
}

type Alternative struct {
	InnerText string `json:",innerxml"`
}

/*
	example query for 'dow chemical'

   "assumptions": {
       "type": "Clash",
       "word": "dow chemical",
       "template": "Assuming \"${word}\" is ${desc1}. Use as ${desc2} instead",
       "count": 2,
       "values": [
           {
               "name": "Financial",
               "desc": "a financial entity",
               "input": "*C.dow+chemical-_*Financial-"
           },
           {
               "name": "Company",
               "desc": "a company",
               "input": "*C.dow+chemical-_*Company-"
           }
       ]
   },
*/

// Assumptions list assumptions made in query result, typically about the meaning of a query or phrase.
type Assumptions struct {
	Assumption []Assumption `json:"assumption"`
	Count      int          `json:"count"`
}

// UnmarshalJSON for assumptions.   Issue with the response from WA in that if a single assumption then an object is returned
//	containing the single assumption detail.   This appears to be a configuration with Ajax java library.   Questionable design
//	but we work around by performing the check.
//  see: https://www.calhoun.io/how-to-parse-json-that-varies-between-an-array-or-a-single-item-with-go/
func (a *Assumptions) UnmarshalJSON(data []byte) error {

	if len(data) == 0 {
		return errors.New("no bytes in assumptions to unmarshall")
	}

	fmt.Printf("\n\n^^^^^^^^\n ******* \nassumptions: %s\n\n", string(data))

	// determine whether object or array and unmarshall appropriately.   Note that go json unmarshaller should have removed
	//	the leading spaces and this should be ok (will fail otherwise).
	switch data[0] {
	case '{':
		fmt.Printf("\n\nassumptions (object: %s\n\n", string(data))
		// unmarshal single assumption
		a.Count = 1
		a.Assumption = make([]Assumption, 1)
		return json.Unmarshal(data, &a.Assumption[0])

	case '[':
		fmt.Printf("\n\nassumptions (array: %s\n\n", string(data))

		if err := jsonLib.Unmarshal(data, &a.Assumption); err != nil {
			return errors.WithMessage(err, "error interpreting assumption")
		} else {
			a.Count = len(a.Assumption)
		}

	default:
		return errors.Errorf("assumptions json does not indicate an object or array (%s)", string(data[0]))
	}
	return nil
}

type Assumption struct {
	Values   []Value `json:"values"`   // alternate values and actions to refine on request
	Type     string  `json:"type"`     // classification of an assumption that defines how it will function
	Word     string  `json:"word"`     // the central word/phrase to which the assumption is applied
	Template string  `json:"template"` // statement outlining the way an assumption will be applied
	Count    int     `json:"count"`
}

/* ForActionDisplay will return a display representation of the assumption with associated action.

	See https://products.wolframalpha.com/api/documentation for detail RE the API

e.g.
	{
		"type":"Clash",
		"word":"dow chemical",
		"template":"Assuming \"${word}\" is ${desc1}. Use as ${desc2} instead",
		"count":2,
		"values":[
			{
				"name":"Financial",
				"desc":"a financial entity",
				"input":"*C.dow+chemical-_*Financial-"
			},
			{
				"name":"Company",
				"desc":"a company",
				"input":"*C.dow+chemical-_*Company-"
			}
		]
	}

	to display:  Assuming "dow chemical" is a financial entity | Use as a company instead
*/

// ActionAssumption represents an assumption that can be displayed and acted upon (switch meaning)
type ActionAssumption struct {
	Label       string
	Action      string // the string to append to request with prop assumption
	ButtonLabel string // the name to use as a button label
	Description string // description (e.g. as Movie)
}

// ForActionDisplay will return a display representation of the assumption with associated action.
func (assumption *Assumption) ForActionDisplay() (*[]ActionAssumption, error) {
	if len(assumption.Values) < 2 {
		// the first element of the assumption list is the one applied.   There has to be >1 for it to be an assumption
		return nil, errors.New("nothing to assume")
	}
	actions := make([]ActionAssumption, 0, len(assumption.Values)-1)
	assumedValue := assumption.Values[0].Description

	template := fasttemplate.New(assumption.Template, "${", "}")

	for _, value := range assumption.Values[1:] {
		var displayAssumption ActionAssumption
		// replace ${word} with assumption word, and ${desc1} and ${desc2}.   desc1 being the assumed value (first element)
		//	and desc2 being the current proposed value.
		label := template.ExecuteString(
			map[string]interface{}{
				"word":  assumption.Word,
				"desc1": assumedValue,
				"desc2": value.Description,
			},
		)

		displayAssumption.Label = label
		displayAssumption.Action = value.Input
		displayAssumption.ButtonLabel = value.Name
		displayAssumption.Description = value.Description
		actions = append(actions, displayAssumption)
	}

	return &actions, nil
}

// Value contains info about an assumption
type Value struct {
	Name        string `json:"name"`
	Description string `json:"desc"`
	Input       string `json:"input"`
}

// Pod elements are sub-elements of <queryresult>. Each contains the results for a single pod
type Pod struct {
	//The subpod elements of the pod
	SubPods []SubPod `json:"subpods"`

	// Infos  []Info  `json:"infos"`	// johnha: api denotes a 'count' property, but missing in actual response (has object with property 'units' for example when looking up UK)

	// states will contain alternative states for the pod (shown as buttons on the wolfram detailed response).  An example is to request more population detail for example.   The state has
	//	a name (that can be displayed as a button), and an 'input'.   This 'input' key can be specified as the 'podstate' parameter.  This field is not URL encoded and will be required to URL
	//	encode if you want to obtain this detail.
	States []State `json:"states"`

	//The pod title, used to identify the pod.
	Title string `json:"title"`

	//The name of the scanner that produced this pod. A guide to the type of
	//data it holds.
	Scanner string `json:"scanner"`

	//Marks the pod that displays the closest thing to a simple "answer" that Wolfram|Alpha can provide
	// Primary bool `json:"primary,omitempty"`

	// true or false depending on whether a serious processing error occurred with this specific pod. If true, there will be an <error> subelement
	Error bool `json:"error"`

	// A number indicating the intended position of the pod in a visual display. These numbers are typically multiples of 100, and they form an increasing sequence from top to bottom.
	Position int `json:"position"`

	// A unique identifier for a pod, used for selecting specific pods to include or exclude.
	ID string `json:"id"`

	//  The number of subpod elements present.
	NumSubPods int `json:"numsubpods"`

	// Sounds     Sounds `json:"sounds"`
}

//If there was a sound related to the query, if you for example query a musical note
//You will get a <sound> element which contains a link to the sound
type Sounds struct {
	Count int     `json:"count"`
	Sound []Sound `json:"sound"`
}

type Sound struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

type Info struct {
	Text string `json:"text"`
	Img  []Img  `json:"img"`
	Link []Link `json:"link"`
}

type Link struct {
	URL   string `json:"url"`
	Text  string `json:"text"`
	Title string `json:"title"`
}

//Each Source contains a link to a web page with the source information
type Sources struct {
	Count  int      `json:"count"`
	Source []Source `json:"source"`
}

type Source struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

// State denotes a refinement of pod detail.  A query will result in pods that have have more detail (states) that can be
//	refined.  The 'name' is the button on wolfram alpha.  The 'input' is a non-url encoded value that can be specified as
//	a podstate prop in additional request (so will need url encoding).
type State struct {
	Name  string `json:"name"`
	Input string `json:"input"` // n.b the 'podstate' prop and non URL endoded to refine detail for a pod.
}

type SubPod struct {
	//HTML <img> element
	Image Img `json:"img"`

	//Textual representation of the subpod
	Plaintext string `json:"plaintext"`

	//Usually an empty string because most subpod elements don't have a title
	Title string `json:"title"`
}

/*
	HTML <img> elements suitable for direct inclusion in a webpage. They point to stored image files giving a formatted visual representation of a single subpod.
	They only appear in pods if the requested result formats include img. In most cases, the image will be in GIF format, although in a few cases it will be in
	JPEG format. The filename in the <img> URL will tell you whether it is GIF or JPEG. The <img> tag also contains the following attributes:

	src — The exact URL of the image being displayed, to be used for displaying the image.
	alt — Alternate text to display in case the image does not render correctly—usually the same as the <plaintext> representation of the image.
	title — Descriptive title for internal identification of an image—usually the same as the <plaintext> representation of the image.
	width — The width of the image in pixels; can be changed using the width control parameters.
	height: — The height of the image in pixels; scales depending on width setting.
*/
type Img struct {
	Src         string `json:"src"`
	Alt         string `json:"alt"`
	Title       string `json:"title"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ContentType string `json:"contenttype"`
}

// GetQueryResult gets the query result from the API and returns it.
// Example extra parameter: "format=image", for a url.Value it'd be:
// u := url.Values{}
// u.Add("format", "image")
// Additional information about parameters can be found at
// http://products.wolframalpha.com/docs/WolframAlpha-API-Reference.pdf, page 42
func (c *Client) GetQueryResult(query string, params url.Values) (*QueryResult, error) {
	query = url.QueryEscape(query)

	url := fmt.Sprintf("https://api.wolframalpha.com/v2/query?input=%s&appid=%s&output=JSON", query, c.AppID)
	if params != nil {
		url += "&" + params.Encode()
	}

	res, err := http.Get(url)
	if err != nil {
		return nil, errors.WithMessage(err, "error in wolfram alpha http request")
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.WithMessage(err, "error in obtaining full wolfram alpha http result")
	}

	// todo: remove json dump of result
	jsonResult, _ := PrettyJsonFromRaw((*json.RawMessage)(&body))
	fmt.Printf("*********\nGetQueryResult JSON\n%s\n", jsonResult)

	data := &Query{}
	data.Result.Query = query

	if err = jsonLib.Unmarshal(body, &data); err != nil {
		return nil, errors.WithMessage(err, "unable to interpret wolfram alpha json result")
	}

	return &data.Result, err
}

// Gets the json from the API and assigns the data to the target.
// The target being a QueryResult struct
func unmarshal(body *http.Response, target interface{}) error {
	defer body.Body.Close()
	return json.NewDecoder(body.Body).Decode(target)
}

// GetSimpleQuery gets an image from the `simple` endpoint.
//
// Returns the image as a response body, the query url, and an error
//
// Can take some extra parameters, e.g `background=F5F5F5`
// sets the background color to #F5F5F5
//
// The rest of the parameters can be found here https://products.wolframalpha.com/simple-api/documentation/
func (c *Client) GetSimpleQuery(query string, params url.Values) (io.ReadCloser, string, error) {
	query = url.QueryEscape(query)

	query = fmt.Sprintf("http://api.wolframalpha.com/v1/simple?appid=%s&input=%s&output=json", c.AppID, query)
	if params != nil {
		query += "&" + params.Encode()
	}

	res, err := http.Get(query)

	if err != nil {
		return nil, "", err
	}

	return res.Body, query, err
}

type Unit int

const (
	Imperial Unit = iota
	Metric
)

func (c *Client) GetShortAnswerQuery(query string, units Unit, timeout int) (string, error) {
	query = url.QueryEscape(query)

	switch units {
	case Imperial:
		query += "&units=imperial"
	case Metric:
		query += "&units=metric"
	}

	if timeout != 0 {
		query += "&timeout=" + strconv.Itoa(timeout)
	}
	query = fmt.Sprintf("https://api.wolframalpha.com/v1/result?appid=%s&i=%s&output=json", c.AppID, query)
	res, err := http.Get(query)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) GetSpokenAnswerQuery(query string, units Unit, timeout int) (string, error) {
	query = url.QueryEscape(query)

	switch units {
	case Imperial:
		query += "&units=imperial"
	case Metric:
		query += "&units=metric"
	}

	if timeout != 0 {
		query += "&timeout=" + strconv.Itoa(timeout)
	}
	query = fmt.Sprintf("https://api.wolframalpha.com/v1/spoken?appid=%s&i=%s&output=json", c.AppID, query)
	res, err := http.Get(query)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type Mode int

const (
	Default Mode = iota
	Voice
)

type FastQueryResult struct {
	Version            string `json:"version"`
	SpellingCorrection string `json:"spellingCorretion"`
	BuildNumber        string `json:"buildnumber"`
	Query              []*struct {
		I                       string `json:"i"`
		Accepted                string `json:"accepted"`
		Timing                  string `json:"timing"`
		Domain                  string `json:"domain"`
		ResultSignificanceScore string `json:"resultsignificancescore"`
		SummaryBox              *struct {
			Path string `json:"path"`
		} `json:"summarybox"`
	} `json:"query"`
}

func (c *Client) GetFastQueryRecognizer(query string, mode Mode) (*FastQueryResult, error) {
	query = url.QueryEscape(query)

	switch mode {
	case Default:
		query += "&mode=Default"
	case Voice:
		query += "&mode=Voice"
	}

	query = fmt.Sprintf(
		"https://www.wolframalpha.com/queryrecognizer/query.jsp?appid=%s&i=%s&output=json", c.AppID, query,
	)

	res, err := http.Get(query)
	if err != nil {
		return nil, err
	}

	qres := &FastQueryResult{}
	err = unmarshal(res, qres)
	if err != nil {
		return nil, err
	}
	return qres, nil
}

// PrettyJsonFromRaw returns a formatted JSON string from raw JSON value
func PrettyJsonFromRaw(bJson *json.RawMessage) (string, error) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, *bJson, "", "    "); err != nil {
		return "", err
	}
	return prettyJSON.String(), nil
}
