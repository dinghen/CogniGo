package image

import (
	"github.com/dinghen/CogniGo/common/image"
	"github.com/dinghen/CogniGo/config"
	"io"
	"log"
	"mime/multipart"
)

func RecognizeImage(file *multipart.FileHeader) (string, error) {

	conf := config.GetConfig()
	libraryPath := conf.RuntimeConfig.ONNXLibraryPath
	modelPath := conf.RuntimeConfig.ModelPath
	labelPath := conf.RuntimeConfig.LabelsPath
	inputH, inputW := 224, 224

	recognizer, err := image.NewImageRecognizer(libraryPath, modelPath, labelPath, inputH, inputW)
	if err != nil {
		log.Println("NewImageRecognizer fail err is : ", err)
		return "", err
	}
	defer recognizer.Close()

	src, err := file.Open()
	if err != nil {
		log.Println("file open fail err is : ", err)
		return "", err
	}
	defer src.Close()

	buf, err := io.ReadAll(src)
	if err != nil {
		log.Println("io.ReadAll fail err is : ", err)
		return "", err
	}

	return recognizer.PredictFromBuffer(buf)
}
