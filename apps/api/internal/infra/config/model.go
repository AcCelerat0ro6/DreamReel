package config

// Config模型 保存基础配置。
type Config struct {
	Port     int            `yaml:"port"`
	Mode     string         `yaml:"mode"` // gin 运行模式: debug / release / test，留空则遵循 GIN_MODE 环境变量
	JWT      JWTConfig      `yaml:"jwt"`
	Internal InternalConfig `yaml:"internal"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	Logger   LoggerConfig   `yaml:"log"`
}

// JWTConfig 保存 JWT 签名密钥和访问 token 有效期。
type JWTConfig struct {
	Secret    string `yaml:"secret"`
	AccessTTL string `yaml:"access_ttl"`
}

// InternalConfig 保存内部服务鉴权配置。
type InternalConfig struct {
	Token string `yaml:"token"`
}

// DatabaseConfig 保存 MySQL 连接参数。
type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Name            string `yaml:"name"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime string `yaml:"conn_max_idle_time"`
}

// RedisConfig 保存 Redis 连接参数。
type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
	DialTimeout  string `yaml:"dial_timeout"`
	ReadTimeout  string `yaml:"read_timeout"`
	WriteTimeout string `yaml:"write_timeout"`
	PoolTimeout  string `yaml:"pool_timeout"`
}

// RabbitMQConfig 保存 RabbitMQ 连接参数。
type RabbitMQConfig struct {
	URL                      string `yaml:"url"`
	InteractionExchange      string `yaml:"interaction_exchange"`
	ActionChangedQueue       string `yaml:"action_changed_queue"`
	ActionChangedRouting     string `yaml:"action_changed_routing"`
	VideoExchange            string `yaml:"video_exchange"`
	VideoPublishedQueue      string `yaml:"video_published_queue"`
	VideoEmbeddingQueue      string `yaml:"video_embedding_queue"`
	VideoPublishedRouting    string `yaml:"video_published_routing"`
	ExposureExchange         string `yaml:"exposure_exchange"`
	ViewEventRecordedQueue   string `yaml:"view_event_recorded_queue"`
	ViewEventRecordedRouting string `yaml:"view_event_recorded_routing"`
}

// LoggerConfig 保存日志配置。
type LoggerConfig struct {
	Level      string `yaml:"level"`
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"max_size"`
	MaxAge     int    `yaml:"max_age"`
	MaxBackups int    `yaml:"max_backups"`
}
