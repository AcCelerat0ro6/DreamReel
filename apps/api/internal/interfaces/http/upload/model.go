package interfacehttpupload

const (
	maxUploadBytes          = 1024 << 20
	maxVideoBytes           = 512 << 20
	maxImageBytes           = 20 << 20
	maxVideoDurationSeconds = 10 * 60
	maxVideoDimension       = 3840
	sniffBytes              = 512
)

type uploadValidation struct {
	Ext  string
	MIME string
}
