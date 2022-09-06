// export two method
package baidu

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emirpasic/gods/sets/treeset"
	"go.uber.org/ratelimit"
)

type SetMeal uint

const (
	Standard SetMeal = iota
	Senior
	Premium
)

type BaiduTranslate struct {
	Appid      string
	Secret     string
	qps        int
	queryLimit int
	ratelimit  ratelimit.Limiter
	jobChan    chan translateJob
}

type BaiduTransEnToZh struct {
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Src       string    `gorm:"primaryKey"`
	Dst       string
}

type FanyiResult map[string]string

type BaiDuTranslateResule struct {
	From         string        `json:"from"`
	To           string        `json:"to"`
	Trans_result []FanyiResult `json:"trans_result"`
	Error_code   string        `json:"error_code"`
	Error_msg    string        `json:"error_msg"`
}

type translateJob struct {
	querys    []string
	backQueue chan []string
	mutex     *sync.Mutex
}

var (
	client          *http.Client
	BaiduTranslater BaiduTranslate
	QueryChan       chan string //独立的 query
	MergeQueryChan  chan string //合并后的 query

)

func init() {
	j, _ := cookiejar.New(nil)

	var PTransport = &http.Transport{Proxy: nil}
	client = &http.Client{
		Transport: PTransport,
		Timeout:   time.Duration(30 * time.Second),
		Jar:       j,
	}
	QueryChan = make(chan string, 1000)
	MergeQueryChan = make(chan string, 30)

}

func NewBaiduTranslater(Appid, Secret string, setmeal SetMeal) *BaiduTranslate {
	var qps, queryLimit int

	// setmeal select
	// https://api.fanyi.baidu.com/doc/8
	switch setmeal {
	case Standard:
		qps = 1
		queryLimit = 1000
	case Senior:
		qps = 10
		queryLimit = 6000
	case Premium:
		qps = 100
		queryLimit = 6000
	}
	rl := ratelimit.New(qps)

	jobChan := make(chan translateJob, 100)
	BaiduTranslater = BaiduTranslate{Appid, Secret, qps, queryLimit, rl, jobChan}
	go BaiduTranslater.translateServer()
	return &BaiduTranslater
}

// 请求翻译，结果保存数据库
func (c *BaiduTranslate) request(query string) *BaiDuTranslateResule {
	salt := strconv.Itoa(rand.Intn(10000000))
	sign := md5V(c.Appid + query + salt + c.Secret)
	v := url.Values{}
	v.Set("q", query)
	v.Set("from", "en")
	v.Set("to", "zh")
	v.Set("appid", c.Appid)
	v.Set("salt", salt)
	v.Set("sign", sign)
	body := ioutil.NopCloser(strings.NewReader(v.Encode())) //把form数据编下码
	reqest, err := http.NewRequest("POST", "http://api.fanyi.baidu.com/api/trans/vip/translate", body)
	if err != nil {
		log.Println(err)
	}
	reqest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(reqest)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close() //一定要关闭resp.Body
	data, _ := ioutil.ReadAll(resp.Body)
	var ret BaiDuTranslateResule
	json.Unmarshal(data, &ret)
	if ret.Error_code != "" {
		log.Println("baidu api query error:", ret.Error_code, ret.Error_msg)
	}
	return &ret
}

// translate One query
func (c *BaiduTranslate) TranslateOne(query string) string {
	c.ratelimit.Take()
	ret := c.request(query)
	retString := ""
	for _, obj := range ret.Trans_result {
		retString += obj["dst"]
	}
	return retString
}

// translate mulit word, rsync
// filter \n
func (c *BaiduTranslate) Translate(querys []string) []string {
	var mutex sync.Mutex
	mutex.Lock()
	retChan := make(chan []string, 1)
	filterQuerus := make([]string, len(querys))
	for i := range querys {
		filterQuerus[i] = strings.ReplaceAll(querys[i], "\n", "")
	}
	c.jobChan <- translateJob{querys: filterQuerus, backQueue: retChan, mutex: &mutex}
	// 等待下一步翻译结束
	mutex.Lock()
	rets := <-retChan
	return rets
}

// 异步翻译，结果保存数据库
func (c *BaiduTranslate) translateServer() {
	// go c.listenTranslate()
	waitMillisecond := int(math.Ceil(float64(1000) / float64(c.qps)))
	for {
		timeout := time.After(time.Millisecond * time.Duration(waitMillisecond))
		jobSlice := []translateJob{}

		for {
			select {
			case <-timeout:
				if len(jobSlice) > 0 {
					// 一次 query,对于单条超过长度的直接返回空值，对于组合超过长度的，拆分查询
					// 接受一波查询，查询完，再一波全部返回，逻辑简单了

					// 结果总存储
					translateDict := make(map[string]string)

					// 去重
					querySet := treeset.NewWithStringComparator()
					for i := range jobSlice {
						for _, queryItem := range jobSlice[i].querys {
							if len(queryItem) > c.queryLimit {
								translateDict[queryItem] = "长度超过限制"
							} else {
								querySet.Add(queryItem)
							}
						}
					}
					// 批量查询
					mergeQuerys := []string{}
					bytes_coun := 0
					for _, queryInterface := range querySet.Values() {
						queryItem := queryInterface.(string)
						if bytes_coun+len(queryItem)+2 <= c.queryLimit {
							// 加入队列
							mergeQuerys = append(mergeQuerys, queryItem)
							bytes_coun += len(queryItem) + 2

						} else if bytes_coun+len(queryItem)+2 > c.queryLimit {
							// 一组内超过长度，立即处理当前的
							queryStrings := strings.Join(mergeQuerys, "\n")
							translateRets := c.request(queryStrings)
							for _, obj := range translateRets.Trans_result {
								translateDict[obj["src"]] = obj["dst"]
							}
							mergeQuerys = []string{queryItem}
						}
					}
					if len(mergeQuerys) > 0 {
						queryStrings := strings.Join(mergeQuerys, "\n")
						translateRets := c.request(queryStrings)
						for _, obj := range translateRets.Trans_result {
							translateDict[obj["src"]] = obj["dst"]
						}
					}

					// 返回任务
					for i := range jobSlice {
						translateRet := make([]string, len(jobSlice[i].querys))
						for j := range jobSlice[i].querys {
							translateRet[j] = translateDict[jobSlice[i].querys[j]]
						}
						jobSlice[i].backQueue <- translateRet
						close(jobSlice[i].backQueue)
						jobSlice[i].mutex.Unlock()
					}
					jobSlice = []translateJob{}
				}
				timeout = time.After(time.Millisecond * time.Duration(waitMillisecond))
			case jobs := <-c.jobChan:
				jobSlice = append(jobSlice, jobs)
			}
		}
	}
}

// 计算 md5
func md5V(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}
