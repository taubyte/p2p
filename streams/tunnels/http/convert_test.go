package httptun

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestConvert(t *testing.T) {
	go http.ListenAndServe("127.0.0.1:2222", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//r := r.Clone(context.Background())

		// for k, v := range r.Header {
		// 	fmt.Println(k, v)
		// 	r2.Header[k] = v

		// }

		body := r.Body

		r.Body = http.NoBody

		//r2.Write(os.Stdout)
		//fmt.Println("---")
		r.Write(os.Stdout)

		fmt.Println("---------")
		defer body.Close()
		io.Copy(os.Stdout, body)

		//http.DefaultClient

	}))

	time.Sleep(3 * time.Second)

	buf := bytes.NewBuffer([]byte("hello y'all"))

	req, err := http.NewRequest("POST", "http://127.0.0.1:2222", buf)
	if err != nil {
		t.Error(err)
		return
	}

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
		return
	}

}
