package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/datty/pam-azuread/internal/conf"

	nss "github.com/protosam/go-libnss"
	nssStructs "github.com/protosam/go-libnss/structs"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
)

// app name
const app = "nss-azuread"

// Placeholder main() stub is neccessary for compile.
func main() {}

func init() {
	// We set our implementation to "LibNssOauth", so that go-libnss will use the methods we create
	nss.SetImpl(LibNssOauth{})
}

// LibNssExternal creates a struct that implements LIBNSS stub methods.
type LibNssOauth struct{ nss.LIBNSS }

var config *conf.Config

func (self LibNssOauth) oauth_init() (result confidential.AuthResult, err error) {

	//Load config vars
	if config == nil {
		if config, err = conf.ReadConfig(); err != nil {
			log.Println("unable to read configfile:", err)
			return result, err
		}
	}

	//Enable oauth cred cache
	cacheAccessor := &TokenCache{"/var/tmp/" + app + "_cache.json"}

	//Attempt oauth
	cred, err := confidential.NewCredFromSecret(config.ClientSecret)
	if err != nil {
		log.Fatal(err)
	}
	app, err := confidential.New(config.ClientID, cred, confidential.WithAuthority("https://login.microsoftonline.com/"+config.TenantID), confidential.WithAccessor(cacheAccessor))
	if err != nil {
		log.Fatal(err)
	}
	result, err = app.AcquireTokenSilent(context.Background(), config.NssScopes)
	if err != nil {
		result, err = app.AcquireTokenByCredential(context.Background(), config.NssScopes)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Access Token Is " + result.AccessToken)
		return result, err
	}
	log.Println("Silently acquired token " + result.AccessToken)
	return result, err

}

//Request against Microsoft Graph API using token, return JSON
func (self LibNssOauth) msgraph_req(t string, req string) (output map[string]interface{}, err error) {

	requestURL := fmt.Sprintf("https://graph.microsoft.com:443/%s", req)
	token := fmt.Sprintf("Bearer %s", t)

	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	request.Header.Set("Authorization", token)
	request.Header.Set("ConsistencyLevel", "eventual")
	if err != nil {
		return output, err
	}
	res, err := http.DefaultClient.Do(request)
	if err != nil {
		return output, err
	}
	//Check if valid response
	if res.StatusCode != 200 {
		return output, fmt.Errorf("%v", res.StatusCode)
	}
	//Close output I guess???
	if res.Body != nil {
		defer res.Body.Close()
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	jsonErr := json.Unmarshal([]byte(body), &output)
	if jsonErr != nil {
		log.Fatal(err)
	}
	return output, nil
}

// PasswdAll will populate all entries for libnss
func (self LibNssOauth) PasswdAll() (nss.Status, []nssStructs.Passwd) {

	//Enable Debug Logging - REMOVE ME! ----------------
	f, err := os.OpenFile("/var/log/"+app+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)
	//Enable Debug Logging - REMOVE ME! ----------------

	//Get OAuth token
	result, err := self.oauth_init()
	log.Println("Test output %s", result)
	if err != nil {
		log.Println("Oauth Failed:", err)
		return nss.StatusUnavail, []nssStructs.Passwd{}
	}

	//Build all users query. Filters users without licences.
	getUserQuery := "/users?$filter=assignedLicenses/$count+ne+0&$count=true&$select=id,userPrincipalName"
	if config.UseSecAttributes {
		//Uses 'beta' endpoint as customSecurityAttributes are only available there.
		getUserQuery = "beta" + getUserQuery + ",customSecurityAttributes"
		log.Println("Query: %s", getUserQuery) //DEBUG
	} else {
		getUserQuery = "v1.0" + getUserQuery + "," + config.UserUIDAttribute + "," + config.UserGIDAttribute
		log.Println("Query: %s", getUserQuery) //DEBUG
	}
	jsonOutput, err := self.msgraph_req(result.AccessToken, getUserQuery)
	if err != nil {
		log.Println("MSGraph request failed:", err)
		return nss.StatusUnavail, []nssStructs.Passwd{}
	}

	//Open Slice/Struct for result
	passwdResult := []nssStructs.Passwd{}

	for _, userResult := range jsonOutput["value"].([]interface{}) {
		//Create temporary struct for user info
		tempUser := nssStructs.Passwd{}
		//Create error capture val
		userErr := false

		//Map value var to correct type to allow for access
		xx := userResult.(map[string]interface{})

		//Get UID/GID
		if config.UseSecAttributes {
			//Set variables ready...not sure if there's a better way to handle this.
			var userSecAttributes map[string]interface{}
			var attributeSet map[string]interface{}
			//Check whether CSA exists
			if xx["customSecurityAttributes"] != nil {
				userSecAttributes = xx["customSecurityAttributes"].(map[string]interface{})
				if userSecAttributes != nil {
					attributeSet = userSecAttributes[config.AttributeSet].(map[string]interface{})
				} else {
					log.Println("No CSA-AS")
					userErr = true
				}
			} else {
				log.Println("No CSA")
				userErr = true
			}
			if attributeSet[config.UserUIDAttribute] != nil {
				//UID exists
				tempUser.UID = uint(attributeSet[config.UserUIDAttribute].(float64))
			} else {
				//Do UID generation magic - but not yet
				userErr = true
			}
			if attributeSet[config.UserGIDAttribute] != nil {
				//GID exists
				tempUser.GID = uint(attributeSet[config.UserGIDAttribute].(float64))
			} else {
				//Set GID to 100 if unset
				tempUser.GID = 100
			}
		} else {
			if xx[config.UserUIDAttribute] != nil {
				tempUser.UID = xx[config.UserUIDAttribute].(uint)
			} else {
				userErr = true
			}
			if xx[config.UserGIDAttribute] != nil {
				tempUser.GID = xx[config.UserGIDAttribute].(uint)
			} else {
				//Set GID to 100 if unset
				tempUser.GID = 100
			}
		}
		//Strip domain from UPN
		user := strings.Split(xx["userPrincipalName"].(string), "@")[0]

		//Set user info
		tempUser.Username = user
		tempUser.Password = "x"
		tempUser.Gecos = app
		tempUser.Dir = fmt.Sprintf("/home/%s", user)
		tempUser.Shell = "/bin/bash"
		if userErr == false {
			passwdResult = append(passwdResult, tempUser)
		}
	}

	return nss.StatusSuccess, passwdResult
}

// PasswdByName returns a single entry by name.
func (self LibNssOauth) PasswdByName(name string) (nss.Status, nssStructs.Passwd) {

	//Enable Debug Logging - REMOVE ME! ----------------
	f, err := os.OpenFile("/var/log/"+app+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)
	//Enable Debug Logging - REMOVE ME! ----------------

	//Get OAuth token
	result, err := self.oauth_init()
	log.Println("Test output %s", result)
	if err != nil {
		log.Println("Oauth Failed:", err)
	}

	// Azure User Lookup - Disable for now.
	//getUserQuery := fmt.Sprintf("v1.0/users/%s?$select=id,displayName,customSecurityAttributes", fmt.Sprintf(config.Domain, name))
	//jsonOutput, err := self.msgraph_req(result.AccessToken, getUserQuery)

	if err != nil {
		log.Println("unable to create user output:", err)
		return nss.StatusNotfound, nssStructs.Passwd{}
	}
	//Disable function for now.
	return nss.StatusNotfound, nssStructs.Passwd{}
}

// PasswdByUid returns a single entry by uid, not managed here
func (self LibNssOauth) PasswdByUid(uid uint) (nss.Status, nssStructs.Passwd) {
	return nss.StatusNotfound, nssStructs.Passwd{}
}

// GroupAll returns all groups
func (self LibNssOauth) GroupAll() (nss.Status, []nssStructs.Group) {
	//Get OAuth token
	result, err := self.oauth_init()
	log.Println("Test output %s", result)
	if err != nil {
		log.Println("Oauth Failed:", err)
	}

	// Azure User Lookup URL
	//graphUrl := fmt.Sprintf("v1.0/groups")
	//Pull all groups from Azure
	//json, err := self.msgraph_req(result.AccessToken, graphUrl)
	if err != nil {
		log.Println("Graph API call failed:", err)
	}
	//for _, value := range json["value"].([]interface{}) {
	//Map value var to correct type
	//	xx := value.(map[string]interface{})
	//}
	//Disable for now. Not a hard requirement.
	//return nss.StatusSuccess, []nssStructs.Group{}
	return nss.StatusNotfound, []nssStructs.Group{}
}

// GroupByName returns a group, not managed here
func (self LibNssOauth) GroupByName(name string) (nss.Status, nssStructs.Group) {

	//disable for now.
	return nss.StatusNotfound, nssStructs.Group{}
}

// GroupBuGid retusn group by id, not managed here
func (self LibNssOauth) GroupByGid(gid uint) (nss.Status, nssStructs.Group) {
	// fmt.Printf("GroupByGid %d\n", gid)
	return nss.StatusNotfound, nssStructs.Group{}
}

// ShadowAll return all shadow entries, not managed as no password are allowed here
func (self LibNssOauth) ShadowAll() (nss.Status, []nssStructs.Shadow) {
	// fmt.Printf("ShadowAll\n")
	return nss.StatusSuccess, []nssStructs.Shadow{}
}

// ShadowByName return shadow entry, not managed as no password are allowed here
func (self LibNssOauth) ShadowByName(name string) (nss.Status, nssStructs.Shadow) {
	// fmt.Printf("ShadowByName %s\n", name)
	return nss.StatusNotfound, nssStructs.Shadow{}
}
