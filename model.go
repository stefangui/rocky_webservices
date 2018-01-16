package main

import (
	"os"
	"fmt"
	"time"
	"regexp"
	"net/http"
	"io/ioutil"
	"database/sql"
	"encoding/json"
)

const weatherTable string = "city_weather"
const timeOutSeconds int64 = 3600
const OpenWeatherURL string = "http://api.openweathermap.org/data/2.5/weather"
const AppID string = "xxxxxxxxxxxxxxxxxxxxxxx"

var gopath string
var dbpath string

type CityName struct {  // for Unmarshal HTTP Request Body
	Name    string
}

type CityWeather struct {   // for Database
	Id          int64   // primary key, auto increment
	Name        string  // city name, UNIQUE
	Main        string  // main in weather
	Description string  // description in weather
	Icon        string  // icon in weather
	Wid         int64   // id in weather
	TimeStamp   int64   // timestamp when updating
}

type WeatherReport struct {
	Id      int64       `json:"id"`
	Main    string      `json:"main"`
	Description string  `json:"description"`
	Icon    string      `json:"icon"`
}

type ReportResult struct {  // for HTTP Response
	Weather    []WeatherReport  `json:"weather"`
}


func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}


func init() {
	InitializeDatabase()
}


func InitializeDatabase() {
	gopath = os.Getenv("GOPATH")
	dbpath = gopath + "/bin/weather.db"

	db, err := sql.Open("sqlite3", dbpath)
	defer db.Close()
	checkErr(err)

	createTable := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` varchar(255) NOT NULL DEFAULT ''  UNIQUE, `main` varchar(255) NOT NULL DEFAULT '' , `description` varchar(255) NOT NULL DEFAULT '' , `icon` varchar(255) NOT NULL DEFAULT '' , `wid` integer NOT NULL DEFAULT 0 , `time_stamp` integer NOT NULL DEFAULT 0);", weatherTable)

	_, err = db.Exec(createTable)
	checkErr(err)
}


// For "POST /location"
func AddOneCity(city string) (respCode int, err error) {
	db, err := sql.Open("sqlite3", dbpath)
	defer db.Close()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	queryStr := fmt.Sprintf("SELECT name FROM %s WHERE name=?", weatherTable)
	tmpName := ""
	db.QueryRow(queryStr, city).Scan(&tmpName)

	if tmpName != "" {    // result set is not empty
		respCode = http.StatusConflict   // 409
	} else {
		insertStr := fmt.Sprintf("INSERT INTO %s (`name`, `wid`, `time_stamp`) values (?, ?, ?)", weatherTable)

		stmt, err := db.Prepare(insertStr)
		if err != nil {
			return http.StatusInternalServerError, err
		}

		_, err = stmt.Exec(city, -1, 0)
		if err != nil {
			return http.StatusInternalServerError, err
		}

		respCode = http.StatusCreated   // 201
	}

	return respCode, err
}

func getAllCities() (allCities []string, respCode int, err error) {
	allCities = []string{}

	db, err := sql.Open("sqlite3",dbpath)
	defer db.Close()
	if err != nil {
		return allCities, http.StatusInternalServerError,err
	}

	queryStr := fmt.Sprintf("select name from %s ", weatherTable)
	rows, err := db.Query(queryStr)
	if err != nil {
		return allCities, http.StatusInternalServerError, err
	}

	for rows.Next(){
		var cityName string
		err = rows.Scan(&cityName)
		if err != nil{
			return allCities,http.StatusInternalServerError,err
		}
		allCities = append(allCities,cityName)
	}

	return allCities,http.StatusOK,err

}

//DELETE /location/{name}
func getOneCityWeather(city string)(result *ReportResult,respCode int, err error)  {
	cw := new(CityWeather)
	result = new(ReportResult)

	db, err := sql.Open("sqlite3",dbpath)
	defer db.Close()
	if err != nil{
		return result, http.StatusInternalServerError,err
	}

	cw.Id = 0
	queryStr := fmt.Sprintf("select id,name,main,description,icon,wid,time_stamp from %s where name=?", weatherTable)
	db.QueryRow(queryStr, city).Scan(&cw.Id,&cw.Description,&cw.Icon,&cw.Main,&cw.TimeStamp,&cw.Wid,&cw.Name)
	if cw.Id == 0 {
		return result, http.StatusNotFound,nil
	}

	currentTime := time.Now().UTC().UnixNano()
	passedSeconds := (currentTime - cw.TimeStamp)

	if(passedSeconds > timeOutSeconds) {
		client := &http.Client{}
		url := fmt.Sprintf("%s?q=%s&APPID=%s",OpenWeatherURL,city,AppID)
		request, err := http.NewRequest("GET",url,nil)
		if(err != nil) {
			return result, http.StatusServiceUnavailable, err
		}
		reponse, err := client.Do(request)
		defer reponse.Body.Close()

		if err != nil {
			return result,http.StatusServiceUnavailable,err
		}else {
			body, err := ioutil.ReadAll(reponse.Body)
			if err != nil {
				return result, http.StatusInternalServerError,err
			}
			bodyStr := string(body)
			reg := regexp.MustCompile(`"weather":(.+)`)
			ws := (reg.FindStringSubmatch(bodyStr))[1]

			//convert "weather" string to byte
			tmpBytes := make([]byte, len(ws))
			copy(tmpBytes[:], ws)

			//Unmarshal the bytes to ReportResult.Weather
			var rcds []WeatherReport
			json.Unmarshal(tmpBytes, &rcds)
			result.Weather = rcds

			cw.Wid			= rcds[0].Id
			cw.Main			= rcds[0].Main
			cw.Description	= rcds[0].Description
			cw.Icon        	= rcds[0].Icon
			cw.TimeStamp   	= currentTime

			updateStr := fmt.Sprintf("update %s set  wid=?, main=?, description=?, icon=?, time_stamp=? WHERE name=?", weatherTable)
			stmt,err := db.Prepare(updateStr)
			if err != nil{
				return result, http.StatusInternalServerError, err
			}

			_, err = stmt.Exec(cw.Wid, cw.Main, cw.Description, cw.Icon, cw.TimeStamp, city)
			if err != nil{
				return result, http.StatusInternalServerError, err
			}

		}
	}else {  //if shorter than timeout get the data from database
		var item WeatherReport
		item.Id          = cw.Wid
		item.Main        = cw.Main
		item.Icon        = cw.Icon
		item.Description = cw.Description

		result.Weather = []WeatherReport{item}
	}

	return result, http.StatusOK,nil
}
