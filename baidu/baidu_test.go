package baidu_test

import (
	"os"
	"testing"

	"github.com/ider-zh/go-translater/baidu"
)

func TestTransater(t *testing.T) {
	t.Log("Appid", os.Getenv("Appid"))
	t.Log("Secret", os.Getenv("Secret"))
	baiduTranslate := baidu.NewBaiduTranslater(os.Getenv("Appid"), os.Getenv("Secret"), baidu.Senior, baidu.ZH)

	ret := baiduTranslate.TranslateOne("good night \n god like")
	t.Log(ret)

	ret = baiduTranslate.TranslateOne("河南人")
	t.Log(ret)

	retS := baiduTranslate.Translate([]string{"good night \n god like", "good basic", "tomcat"})
	t.Log(retS)

	retS = baiduTranslate.Translate([]string{"tomcat"})
	t.Log(retS)

	retS = baiduTranslate.Translate([]string{"york", "golang 2021", "tomcat", "facebook", "龙猫"})
	t.Log(retS)
}
