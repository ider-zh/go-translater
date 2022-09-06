package baidu_test

import (
	"os"
	"testing"

	"github.com/ider-zh/go-translater/baidu"
)

func TestTransater(t *testing.T) {
	t.Log("Appid", os.Getenv("Appid"))
	t.Log("Secret", os.Getenv("Secret"))
	baiduTranslate := baidu.NewBaiduTranslater(os.Getenv("Appid"), os.Getenv("Secret"), baidu.Senior)

	ret := baiduTranslate.TranslateOne("good night \n god like")
	t.Log(ret)

	retS := baiduTranslate.Translate([]string{"good night \n god like", "good basic", "tomcat"})
	t.Log(retS)

	retS = baiduTranslate.Translate([]string{"york", "golang 2021", "tomcat", "facebook"})
	t.Log(retS)
}
