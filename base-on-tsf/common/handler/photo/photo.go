/*
   @Time : 2020/5/17 1:45 下午
   @Author : ShadowWalker
   @Email : master@rebeta.cn
   @File : photo
   @Software: GoLand
*/

package photo

import (
	"encoding/base64"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/offcn-jl/go-common/codes"
	"github.com/offcn-jl/go-common/database/orm"
	"github.com/offcn-jl/go-common/logger"
	"github.com/offcn-jl/serverless-apis/base-on-tsf/common/config"
	"github.com/offcn-jl/serverless-apis/base-on-tsf/common/database/orm/structs"
	bda "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/bda/v20200324"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	fmu "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/fmu/v20191213"
	"io/ioutil"
	"net/http"
	"time"
)

func PostHandler(c *gin.Context) {
	// 校验应用授权信息
	if c.GetHeader("Token") == "" {
		// 授权信息不存在 ( 没有在 Header 中设置 Token )
		c.JSON(http.StatusUnauthorized, gin.H{"Code": codes.MissingAuthInfo, "Error": codes.ErrorText(codes.MissingAuthInfo)})
		return
	}
	authInfo := structs.AppAuthInfo{}
	orm.PostgreSQL.Where("token = ?", c.GetHeader("Token")).Find(&authInfo)
	if authInfo.ID == 0 {
		// 授权信息不存在 ( 没有找到对应的 Token )
		c.JSON(http.StatusUnauthorized, gin.H{"Code": codes.NotExistToken, "Error": codes.ErrorText(codes.NotExistToken)})
		return
	}
	duration, _ := time.ParseDuration("-" + fmt.Sprint(authInfo.ExpiresIn) + "s")
	if authInfo.CreatedAt.Before(time.Now().Add(duration)) {
		// 授权已过期
		c.JSON(http.StatusForbidden, gin.H{"Code": codes.NotCertifiedToken, "Error": codes.ErrorText(codes.NotCertifiedToken)})
		return
	}

	logger.Log("开始进行照片处理.")
	// 从请求 Body 中读取 POST 提交的图片二进制 Buffer
	body, _ := ioutil.ReadAll(c.Request.Body)
	// 将图片 Buffer 编码为 Base64
	base64BodyString := base64.StdEncoding.EncodeToString(body)

	// 生成 腾讯云 API SDK 配置信息
	credential := common.NewCredential(config.TencentCloud.APISecretID, config.TencentCloud.SecretKey)
	cpf := profile.NewClientProfile()

	// 人像分割
	logger.Log("人像分割开始.")
	//cpf.HttpProfile.Endpoint = "bda.tencentcloudapi.com"
	// 使用内网地址调用接口
	cpf.HttpProfile.Endpoint = "bda.internal.tencentcloudapi.com"
	client, _ := bda.NewClient(credential, "ap-beijing", cpf)
	request := bda.NewSegmentPortraitPicRequest()
	params := "{\"Image\":\"" + base64BodyString + "\"}"
	err := request.FromJsonString(params)
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
		return
	}
	response, err := client.SegmentPortraitPic(request)
	if _, ok := err.(*errors.TencentCloudSDKError); ok {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
		return
	}
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
		return
	}
	//fmt.Printf("%s", response.ToJsonString())
	logger.Log("人像分割完成.")
	logger.DebugToJson("Response", response)

	base64Image := *response.Response.ResultImage

	logger.Log("美颜参数 ( Beauty ) : " + c.Param("Beauty"))
	if c.Param("Beauty") == "true" {
		// 人脸美颜
		logger.Log("人脸美颜开始.")
		//cpf.HttpProfile.Endpoint = "fmu.tencentcloudapi.com"
		// 使用内网地址调用接口
		cpf.HttpProfile.Endpoint = "fmu.internal.tencentcloudapi.com"
		client, _ := fmu.NewClient(credential, "ap-beijing", cpf)

		request := fmu.NewBeautifyPicRequest()

		params := "{\"Image\":\"" + base64Image + "\",\"Whitening\":50,\"Smoothing\":50,\"FaceLifting\":50,\"EyeEnlarging\":50}"
		err := request.FromJsonString(params)
		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
			return
		}
		response, err := client.BeautifyPic(request)
		if _, ok := err.(*errors.TencentCloudSDKError); ok {
			logger.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
			return
		}
		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
			return
		}
		//fmt.Printf("%s", response.ToJsonString())
		base64Image = *response.Response.ResultImage
		logger.Log("人脸美颜结束.")
		logger.DebugToJson("Response", response)
	}

	// 将 Base64 编码的图片解码为 Buffer
	imageBuffer, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err})
		return
	}

	logger.Log("照片处理完成, 返回 Buffer.")
	// 返回 Buffer
	c.Data(http.StatusOK, "arraybuffer", imageBuffer)
}
