package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"context"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

	"github.com/andybalholm/brotli"
	"github.com/gorilla/mux"
)


type ResponsesError struct {
	Error    	ErrorStruct 	`json:"error"`
}
type ErrorStruct struct{
	Code 		int 			`json:"code"`
	Message 	string 			`json:"message"`
}
type Requestencr struct {
	Req string `json:"RequestData"`
}

type Responseencr struct {
	Res string `json:"ResponseData"`
}
// production-env
var (
	port = ":8000"
	targetURL = "http://xyz"
	Client_url           string
	Application_newUrl   string
	Appl_resp_Statuscode int
	Req_header           string
)

secret Manger for aes key calling
func accessSecret(secretName string) (string, error) {
    ctx := context.Background()

    // Create the client
    client, err := secretmanager.NewClient(ctx)
    if err != nil {
        return "", fmt.Errorf("failed to create secretmanager client: %v", err)
    }
    defer client.Close()

    // Build the request
    req := &secretmanagerpb.AccessSecretVersionRequest{
        Name: secretName, 
    }

    // Access the secret
    result, err := client.AccessSecretVersion(ctx, req)
    if err != nil {
        return "", fmt.Errorf("failed to access secret: %v", err)
    }

    // Extract the secret payload
    payload := string(result.Payload.Data)
    return payload, nil
}


func handle_log(appln_req string, client_resp string, err error) {
	log.SetFlags(0) // Disable default timestamps etc.

	// Create a structured log entry
	logEntry := map[string]interface{}{
		"status_code":   Appl_resp_Statuscode,
		"client_url":    Client_url,
		"error":         fmt.Sprint(err), // handles nil case too
		"request_header": Req_header,
	}

	// Marshal to JSON
	logJSON, jsonErr := json.Marshal(logEntry)
	if jsonErr != nil {
		log.Println("Failed to marshal log entry:", jsonErr)
		return
	}

	// Print JSON log
	log.Println(string(logJSON))
}

func proxyHandler(w http.ResponseWriter, req *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "X-Requested-With,Content-Type,Authorization,Content-Length,Referer,Origin,Accept-Encoding,Content-Length, Content-Disposition, Accept")
	w.Header().Set("Access-Control-Expose-Headers", "*")
	// w.Header().Set("Access-Control-Max-Age", "3628800")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")

	if req.Method == "OPTIONS" {
		w.Header().Set("Content-Type", "*")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Expose-Headers", "*")
		// w.Header().Set("Access-Control-Max-Age", "3628800")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.WriteHeader(http.StatusOK)
		return
	}
	Client_url = req.Host + req.URL.Path
	var res Responseencr
	var application_endpt string
	q := req.URL.Query().Encode()
	if(q != ""){
		application_endpt = req.URL.Path + "?" + q
	}else{
		application_endpt = req.URL.Path
	}
	// fmt.Println("q :", q)
	// fmt.Println("application_endpt :",application_endpt)

	Application_newUrl = targetURL + application_endpt
	// Apiusername = req.Header.Get("apiusername")
	// fmt.Println("Apiusername :", Apiusername)

	key := "abczyx"
	
    // CALL secret manager
	secretFullName := "projects/123/secrets/application/versions/latest"
    key, err2 := accessSecret(secretFullName)
    if err2 != nil {
        fmt.Printf("Error accessing secret: %v\n", err2)
        return
    }


	var decryptedValue interface{}
	var decryptedJson []byte
	var Application_req *http.Request
	var err error

	if req.Method != "GET" {
		var incomingReq Requestencr
		// fmt.Println("Content-Type:=", req.Header.Get("Content-Type"))
		if req.Header.Get("Content-Type") != "" && strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
			fmt.Println("Executed")
			Application_req, err = http.NewRequest(req.Method, Application_newUrl, req.Body)
			if err != nil {
				err = errors.New("Error in preparing request :" + err.Error())
				handle_log("", "", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			Application_req.Header = req.Header
			Application_req.Host = req.Header.Get("Host")
		} else {
			reqBody, err := io.ReadAll(req.Body)
			if err != nil {
				err = errors.New("Error reading request Body: " + err.Error())
				handle_log("", "", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			err = json.Unmarshal(reqBody, &incomingReq)
			if err != nil {
				err = errors.New("Error in unmarshalling incoming request: " + err.Error())
				handle_log("", "", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// fmt.Println("incomingReq.Req",incomingReq.Req )
			if incomingReq.Req == "" {
				Res := map[string]interface{}{
					"error": map[string]interface{}{
						"code":    -1,
						"message": "Encrypted data is not present in the 'RequestData' parameter.",
					},
				}
				json.NewEncoder(w).Encode(Res)
				handle_log("", "Invalid request", nil)
				return
			}

			decryptedValue, err = DecryptRequest(incomingReq.Req, key)
			var responseError ResponsesError
			// fmt.Println("var responseError ResponsesError", " err: ",err)
			if err != nil {
				if strings.Contains(err.Error(), "failed to remove padding:") {
					fmt.Println("failed to remove padding:")
					responseError.Error.Code = -1
					responseError.Error.Message = "Error in the padding or block size in the payload."
					// return false, ResponseError, decr_Data, errors.New(ResponseError.Error.Message)
					jsonData, err := json.Marshal(responseError)
					if err != nil {
						fmt.Println("Error marshalling object to JSON:", err)
						http.Error(w, responseError.Error.Message+ " :failed to marshal the object", http.StatusBadRequest)
						return
					}
					errorString := string(jsonData)
					http.Error(w, errorString, http.StatusBadRequest)
					return
				} else if strings.Contains(err.Error(), "failed to unmarshal JSON:") {
					fmt.Println("failed to unmarshal JSON:")
					responseError.Error.Code = -1
					responseError.Error.Message = "Error in the encryption-decryption key or incorrect JSON request format or there is an illegal argument."
					// return false, ResponseError, decr_Data, errors.New(ResponseError.Error.Message)
					jsonData, err := json.Marshal(responseError)
					if err != nil {
						fmt.Println("Error marshalling object to JSON:", err)
						http.Error(w, responseError.Error.Message+ " :failed to marshal the object", http.StatusBadRequest)
						return
					}
					errorString := string(jsonData)
					http.Error(w, errorString, http.StatusBadRequest)
					return
				}
				fmt.Println("default err")
				// err = errors.New("Error Decrypting incoming request: " + err.Error())
				// handle_log("", "", err)
				// http.Error(w, err.Error(), http.StatusBadRequest)
				// return
				responseError.Error.Code = -1
				responseError.Error.Message = "Error in the encryption-decryption key in the payload."
				// return false, ResponseError, decr_Data, errors.New(ResponseError.Error.Message)
				jsonData, err := json.Marshal(responseError)
				if err != nil {
					fmt.Println("Error marshalling object to JSON:", err)
					http.Error(w, responseError.Error.Message+ " :failed to marshal the object", http.StatusBadRequest)
					return
				}
				errorString := string(jsonData)
				http.Error(w, errorString, http.StatusBadRequest)
				return
			}

			// fmt.Println("decrypted")
			decryptedJson, err = json.Marshal(decryptedValue)
			if err != nil {
				err = errors.New("Error Decrypting incoming request: " + err.Error())
				handle_log("", "", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// fmt.Println("decrypted value:=", decryptedValue)

			Application_req, err = http.NewRequest(req.Method, Application_newUrl, bytes.NewBuffer(decryptedJson))
			if err != nil {
				err = errors.New("Error in preparing request: " + err.Error())
				handle_log(string(decryptedJson), "", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			Application_req.Header = req.Header
			Application_req.Host = req.Header.Get("Host")
		}
	} else { //handled For GET Requests
		Application_req, err = http.NewRequest(req.Method, Application_newUrl, nil)
		if err != nil {
			err = errors.New("Error in preparing request: " + err.Error())
			handle_log("", "", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		Application_req.Header = req.Header
		Application_req.Host = req.Header.Get("Host")
	}
	application_client := http.Client{}
	Application_req.Header = req.Header
	Application_req.Host = req.Header.Get("Host")
	Application_resp, err := application_client.Do(Application_req)
	if err != nil {
		err = errors.New("Error in sending http NewRequest :" + err.Error())
		handle_log(string(decryptedJson), "", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer Application_resp.Body.Close()

	Appl_resp_Statuscode = Application_resp.StatusCode
	header, _ := json.Marshal(req.Header)
	Req_header = string(header)

	var body []byte
	var readErr error

	switch Application_resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err := gzip.NewReader(Application_resp.Body)
		if err != nil {
			handle_log(string(decryptedJson), "", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer reader.Close()
		body, readErr = io.ReadAll(reader)
	case "br":
		reader := brotli.NewReader(Application_resp.Body)
		body, readErr = io.ReadAll(reader)
	default:
		body, readErr = io.ReadAll(Application_resp.Body)
	}

	// fmt.Println("Response Body:", body)
	if readErr != nil {
		err = errors.New("Error reading response: " + readErr.Error())
		handle_log(string(decryptedJson), "", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := Application_resp.Header.Get("Content-Type")
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(Application_resp.StatusCode)

	if Application_resp.StatusCode == http.StatusOK {
		encryptedResponseString, err := EncryptResponse(body, key)
		if err != nil {
			err = errors.New("Error Encrypting Response: " + err.Error())
			handle_log(string(decryptedJson), string(body), err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res.Res = encryptedResponseString
		handle_log(string(decryptedJson), string(body), nil)
		json.NewEncoder(w).Encode(res)
	} else {

		handle_log(string(decryptedJson), string(body), nil)
		w.Write([]byte(body))
	}
}

func main() {
	fmt.Printf("Server started on port %s \n",port)
	// targetURL = strings.TrimSpace(os.Getenv("targetURL"))
	targetURL = strings.TrimSpace(targetURL)
	if(targetURL == ""){
		panic("Exiting no env 'targetURL' set")
	}else{
		fmt.Println("targetURL :", targetURL)
	}
	router := mux.NewRouter()
	router.HandleFunc("/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}", proxyHandler)
	router.HandleFunc("/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}/{obj}", proxyHandler)
	err := http.ListenAndServe(port, router)
	if err != nil {
		fmt.Printf("Error listening at port %s : %s \n",port , err)
	}
}

func EncryptResponse(responseBody []byte, key string) (string, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %w", err)
	}

	if len(keyBytes) != 32 {
		return "", errors.New("invalid key length: must be 32 bytes when decoded")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	var dataToEncrypt []byte
	if json.Valid(responseBody) {
		dataToEncrypt = responseBody
	} else {
		dataToEncrypt, err = json.Marshal(string(responseBody))
		if err != nil {
			return "", fmt.Errorf("failed to marshal non-JSON data: %w", err)
		}
	}

	paddedData := PKCS7Padding(dataToEncrypt, aes.BlockSize)

	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(paddedData))
	mode.CryptBlocks(encrypted, paddedData)

	result := append(iv, encrypted...)

	encodedResult := base64.StdEncoding.EncodeToString(result)
	return encodedResult, nil
}

func PKCS7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func DecryptRequest(encryptedString string, key string) (interface{}, error) {
	decodedData, err := base64.StdEncoding.DecodeString(encryptedString)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted string: %w", err)
	}

	if len(decodedData) < aes.BlockSize {
		return "", errors.New("ciphertext too short")
	}

	iv := decodedData[:aes.BlockSize]
	ciphertext := decodedData[aes.BlockSize:]

	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	plaintext, err = PKCS7Unpadding(plaintext)
	if err != nil {
		return "", fmt.Errorf("failed to remove padding: %w", err)
	}

	var jsonResult interface{}
	if json.Valid(plaintext) {
		err = json.Unmarshal(plaintext, &jsonResult)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		return jsonResult, nil
	}

	return string(plaintext), nil
}
func PKCS7Unpadding(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("invalid padding")
	}
	unpadding := int(data[length-1])
	if unpadding > length {
		return nil, errors.New("invalid padding")
	}
	return data[:(length - unpadding)], nil
}
