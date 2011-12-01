package spamhamdb

import (
	"os"
	"http"
	"template"
	"appengine"
	"appengine/datastore"
	"appengine/user"
	"fmt"
	"io/ioutil"
)

func init() {
	http.HandleFunc("/", index)
	http.HandleFunc("/spamham", spamham)
	http.HandleFunc("/tweet", getTweet)
	http.HandleFunc("/uploadTweet", putTweet)
	http.HandleFunc("/categorize", categorizeTweet)
	http.HandleFunc("/spam", getSpam)
	http.HandleFunc("/ham", getHam)
}



func requireLogin(c appengine.Context, w http.ResponseWriter, returnUrl string) (*user.User){
	u := user.Current(c)
	if u == nil {
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusFound)
		return nil
	}
	return u
}

func index(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u != nil {
		w.Header().Set("Location", "/spamham")
		w.WriteHeader(http.StatusFound)
		return
	}
	loginUrl, _ := user.LoginURL(c, r.URL.String())
	mainPageTemplate, templateErr := template.ParseFile("templates/index.html")
	if templateErr != nil {
		http.Error(w, templateErr.String(), http.StatusInternalServerError)
	}
	err := mainPageTemplate.Execute(w, loginUrl)
  if err != nil {
		http.Error(w, err.String(), http.StatusInternalServerError)
  }
}

func spamham(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	requireLogin(c, w, r.URL.String())
	mainPageTemplate, templateErr := template.ParseFile("templates/spamham.html")
	if templateErr != nil {
		http.Error(w, templateErr.String(), http.StatusInternalServerError)
	}
	err := mainPageTemplate.Execute(w, nil)
  if err != nil {
		http.Error(w, err.String(), http.StatusInternalServerError)
  }
}

func putTweet(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	requireLogin(c, w, r.URL.String())
	if user.IsAdmin(c) {
		body, readErr := ioutil.ReadAll(r.Body)
		if readErr != nil {
			fmt.Fprintf(w, "Read Error: %s", readErr)
			return
		}
		saveErr := saveTweet(c, "tweet", body)
		if saveErr != nil {
			fmt.Fprintf(w, "Save Error: %s", saveErr)
			return
		}
	}
}

func getTweet(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	q := datastore.NewQuery("tweet").Limit(1)
	for t := q.Run(c); ; {
		var x Entity
		key, err := t.Next(&x)
		if err == datastore.Done {
			break
		}
		if err != nil {
			return
		}
		tweetCount, _ := getTweetCount(c)
		fmt.Fprintf(w, "{\"key\": \"%s\", \"value\": %s, \"count\": %v}", key.Encode(), x.Value, tweetCount)
	}
}

func categorizeTweet(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	requireLogin(c, w, r.URL.String())
	key := r.FormValue("key")
	tweetType := r.FormValue("type")
	if len(key) > 0 && len(tweetType) > 0 {
		var e Entity
		k, kErr := datastore.DecodeKey(key)
		if kErr != nil {
			c.Errorf("Error decoding key (%s): %s", key, kErr.String())
		}
		if err := datastore.Get(c, k, &e); err != nil {
			c.Errorf("Error getting key: %s", err.String())
			http.Error(w, err.String(), http.StatusInternalServerError)
			return
		}
		saveTweetEntity(c, tweetType, e)
		datastore.Delete(c, k)
		decTweetCounter(c)
	}
}

func saveTweet(c appengine.Context, key string, t []byte) (os.Error) {
	e := Entity{ Value: t }
	saveErr := saveTweetEntity(c, key, e)
	if saveErr != nil {
		return saveErr
	}
	return incTweetCounter(c)
}

func saveTweetEntity(c appengine.Context, key string, e Entity)  (os.Error) {
	_, dataErr := datastore.Put(c, datastore.NewIncompleteKey(c, key, nil), &e)
	if dataErr != nil {
		c.Errorf("Error saving tweet: %s", dataErr.String())
		return dataErr
	}
	return nil
}

func incTweetCounter(c appengine.Context) (os.Error) {
	return tweetCountOp(c, func(count *Counter) {
		count.Count++
	})
}

func decTweetCounter(c appengine.Context) (os.Error) {
	return tweetCountOp(c, func(count *Counter) {
		count.Count--
	})
}

func tweetCountOp(c appengine.Context, f func(count *Counter)) (os.Error) {
	key := datastore.NewKey(c, "tweet_counter", "tweet_counter", 0, nil)
	count := new(Counter)
	err := datastore.RunInTransaction(c, func(c appengine.Context) os.Error {
		err := datastore.Get(c, key, count)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		f(count)
		_, dataErr := datastore.Put(c, key, count)
		return dataErr
	}, nil)
	
	if err != nil {
		c.Errorf("Transaction failed: %v", err)
		return err
	}
	return nil
}

func getTweetCount(c appengine.Context) (int, os.Error){
	key := datastore.NewKey(c, "tweet_counter", "tweet_counter", 0, nil)
	count := new(Counter)
	err := datastore.Get(c, key, count)
	return count.Count, err
}

func getHam(w http.ResponseWriter, r *http.Request) {
	printSpamHamJSON(w, r, "ham")
}

func getSpam(w http.ResponseWriter, r *http.Request) {
	printSpamHamJSON(w, r, "spam")
}

func printSpamHamJSON(w http.ResponseWriter, r *http.Request, key string) {
	c := appengine.NewContext(r)
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	q := datastore.NewQuery(key)
	fmt.Fprintf(w, "[")
	first := true
	for t := q.Run(c); ; {
		var x Entity
		_, err := t.Next(&x)
		if err == datastore.Done {
			break
		}
		if err != nil {
			return
		}
		if (!first) {
			fmt.Fprintf(w, ", %s", x.Value)
		} else {
			fmt.Fprintf(w, "%s", x.Value)
			first = false
		}
	}
	fmt.Fprintf(w, "]")
}


type Entity struct {
    Value []byte
}

type Counter struct {
    Count int
}