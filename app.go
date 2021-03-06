/*
 * Copyright (C) 2019  Murat Koptur
 *
 * Contact: mkoptur3@gmail.com
 *
 * Last edit: 4/1/19 12:23 PM
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type App struct {
	Router *mux.Router
	Logger http.Handler
	DB     *sql.DB
}

// curl --user user1:pass1 127.0.0.1:8000/api/products/list
func (a *App) getProducts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query("SELECT * FROM products")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		err := rows.Scan(&p.Id, &p.Name, &p.Manufacturer)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}

		products = append(products, p)
	}

	_ = json.NewEncoder(w).Encode(products)
}

// curl --header "Content-Type: application/json" --request POST --data '{"name": "ABC", "manufacturer": "ACME"}' \
// 		--user user1:pass1 127.0.0.1:8000/api/products/new
func (a *App) createProduct(w http.ResponseWriter, r *http.Request) {
	var p Product
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	defer r.Body.Close()

	_, err := a.DB.Query("INSERT INTO products (name, manufacturer) VALUES (?, ?)", p.Name, p.Manufacturer)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithMessage(w, http.StatusCreated, "New row added.")
}

// curl --user user1:pass1 127.0.0.1:8000/api/products/10
func (a *App) getProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid product ID")
		return
	}

	p := Product{Id: id}
	row := a.DB.QueryRow("SELECT name, manufacturer FROM products WHERE id=?", p.Id)
	if err := row.Scan(&p.Name, &p.Manufacturer); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, p)
}

// curl --request PUT --data '{"name": "ABC", "manufacturer": "ACME"}' --user user1:pass1 127.0.0.1:8000/api/products/11
func (a *App) updateProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid product ID")
		return
	}

	var p Product
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	defer r.Body.Close()
	p.Id = id

	_, err = a.DB.Query("UPDATE products SET name=?, manufacturer=? WHERE id=?", p.Name, p.Manufacturer, p.Id)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, p)
}

// curl --request DELETE --user user1:pass1 127.0.0.1:8000/api/products/10
func (a *App) deleteProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid product ID")
		return
	}

	_, err = a.DB.Query("DELETE FROM products WHERE id=?", id)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithMessage(w, http.StatusOK, "Deleted.")
}

func (a *App) authHandler(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(s) != 2 {
			respondWithError(w, http.StatusUnauthorized, "Invalid/Missing Credentials.")
			return
		}

		b, err := base64.StdEncoding.DecodeString(s[1])
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid/Missing Credentials.")
			return
		}

		pair := strings.SplitN(string(b), ":", 2)
		if len(pair) != 2 {
			respondWithError(w, http.StatusUnauthorized, "Invalid/Missing Credentials.")
			return
		}

		user := User{Username: pair[0]}
		row := a.DB.QueryRow("SELECT id, saltedpassword, salt FROM users WHERE username=?", user.Username)
		if err := row.Scan(&user.Id, &user.Saltedpassword, &user.Salt); err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid/Missing Credentials.")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Saltedpassword), []byte(pair[1]+user.Salt)); err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid/Missing Credentials.")
			return
		}

		f(w, r)
	}
}

func (a *App) InitializeRoutes() {
	a.Router.HandleFunc("/api/products/list", a.authHandler(a.getProducts)).Methods("GET")
	a.Router.HandleFunc("/api/products/new", a.authHandler(a.createProduct)).Methods("POST")
	a.Router.HandleFunc("/api/products/{id:[0-9]+}", a.authHandler(a.getProduct)).Methods("GET")
	a.Router.HandleFunc("/api/products/{id:[0-9]+}", a.authHandler(a.updateProduct)).Methods("PUT")
	a.Router.HandleFunc("/api/products/{id:[0-9]+}", a.authHandler(a.deleteProduct)).Methods("DELETE")
}

func (a *App) Initialize(username, password, server, port, dbName string) {
	dataSource := username + ":" + password + "@tcp(" + server + ":" + port + ")/" + dbName
	a.DB, err = sql.Open("mysql", dataSource)
	if err != nil {
		log.Fatal(err)
	}

	a.Router = mux.NewRouter()
	a.Logger = handlers.CombinedLoggingHandler(os.Stdout, a.Router)
	a.InitializeRoutes()
}

func (a *App) Run(addr string) {
	log.Fatal(http.ListenAndServe(":"+viper.GetString("Server.port"), a.Logger))
}
