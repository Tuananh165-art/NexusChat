package config

import (
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Web           *WebConfig           `mapstructure:"web"`
	Chat          *ChatConfig          `mapstructure:"chat"`
	AI            *AIConfig            `mapstructure:"ai"`
	Forwarder     *ForwarderConfig     `mapstructure:"forwarder"`
	Match         *MatchConfig         `mapstructure:"match"`
	Uploader      *UploaderConfig      `mapstructure:"uploader"`
	User          *UserConfig          `mapstructure:"user"`
	Safety        *SafetyConfig        `mapstructure:"safety"`
	Discovery     *DiscoveryConfig     `mapstructure:"discovery"`
	Workspace     *WorkspaceConfig     `mapstructure:"workspace"`
	Notification  *NotificationConfig  `mapstructure:"notification"`
	Kafka         *KafkaConfig         `mapstructure:"kafka"`
	Cassandra     *CassandraConfig     `mapstructure:"cassandra"`
	Redis         *RedisConfig         `mapstructure:"redis"`
	Observability *ObservabilityConfig `mapstructure:"observability"`
}

type AIConfig struct {
	BaseURL             string
	RequestTimeoutMilli int64
}

type WebConfig struct {
	Http struct {
		Server struct {
			Port string
		}
	}
}

type ChatConfig struct {
	Http struct {
		Server struct {
			Port    string
			MaxConn int64
			Swag    bool
		}
	}
	Grpc struct {
		Server struct {
			Port string
		}
		Client struct {
			User struct {
				Endpoint string
			}
			Forwarder struct {
				Endpoint string
			}
		}
	}
	Subscriber struct {
		Id string
	}
	Message struct {
		MaxNum        int64
		PaginationNum int
		MaxSizeByte   int64
	}
	JWT struct {
		Secret           string
		ExpirationSecond int64
	}
}

type ForwarderConfig struct {
	Grpc struct {
		Server struct {
			Port string
		}
	}
}

type MatchConfig struct {
	Http struct {
		Server struct {
			Port    string
			MaxConn int64
			Swag    bool
		}
	}
	Grpc struct {
		Client struct {
			Chat struct {
				Endpoint string
			}
			User struct {
				Endpoint string
			}
			Safety struct {
				Endpoint string
			}
			Discovery struct {
				Endpoint string
			}
		}
	}
}

type RateLimitConfig struct {
	Rps   int
	Burst int
}

type UploaderConfig struct {
	Http struct {
		Server struct {
			Port          string
			Swag          bool
			MaxBodyByte   int64
			MaxMemoryByte int64
		}
	}
	S3 struct {
		Endpoint              string
		PublicEndpoint        string
		Region                string
		Bucket                string
		AccessKey             string
		SecretKey             string
		PresignLifetimeSecond int64
	}
	RateLimit struct {
		ChannelUpload RateLimitConfig
	}
}

type CookieConfig struct {
	MaxAge   int
	Path     string
	Domain   string
	Secure   bool
	HttpOnly bool
	SameSite string
}

type UserConfig struct {
	Http struct {
		Server struct {
			Port string
			Swag bool
		}
	}
	Grpc struct {
		Server struct {
			Port string
		}
	}
	OAuth struct {
		Cookie CookieConfig
		Google struct {
			RedirectUrl  string
			ClientId     string
			ClientSecret string
			Scopes       string
		}
	}
	Auth struct {
		Cookie CookieConfig
	}
}

type PlatformServiceConfig struct {
	Http struct {
		Server struct {
			Port string
		}
	}
	Grpc struct {
		Server struct {
			Port string
		}
		Client struct {
			Safety struct {
				Endpoint string
			}
			Discovery struct {
				Endpoint string
			}
			Chat struct {
				Endpoint string
			}
		}
	}
}

type SafetyConfig struct {
	PlatformServiceConfig `mapstructure:",squash"`
	WarnScore             int
	BlockScore            int
	RateLimit             struct {
		WindowSecond int64
		MaxMessages  int64
		MuteSecond   int64
	}
	Workers int
}

type DiscoveryConfig struct {
	PlatformServiceConfig `mapstructure:",squash"`
	CandidateLimit        int
	RecentPeerDay         int
	Workers               int
}

type WorkspaceConfig struct {
	PlatformServiceConfig `mapstructure:",squash"`
	Workers               int
	ReminderPollSecond    int64
}

type KafkaConfig struct {
	Addrs   string
	Version string
}

type NotificationConfig struct {
	PollIntervalSecond int64
	LeaseSecond        int64
	MaxAttempts        int
	BaseRetrySecond    int64
	MaxRetrySecond     int64
	BatchSize          int
	WorkerID           string
}

type CassandraConfig struct {
	Hosts    string
	Port     int
	User     string
	Password string
	Keyspace string
}

type RedisConfig struct {
	Password                string
	Addrs                   string
	ExpirationHour          int64
	MinIdleConn             int
	PoolSize                int
	ReadTimeoutMilliSecond  int64
	WriteTimeoutMilliSecond int64
}

type ObservabilityConfig struct {
	Prometheus struct {
		Port string
	}
	Tracing struct {
		OtlpEndpoint string
	}
}

func setDefault() {
	viper.SetDefault("web.http.server.port", "5000")

	viper.SetDefault("chat.http.server.port", "5001")
	viper.SetDefault("chat.http.server.maxConn", 200)
	viper.SetDefault("chat.http.server.swag", false)
	viper.SetDefault("chat.grpc.server.port", "4000")
	viper.SetDefault("chat.grpc.client.user.endpoint", "localhost:4001")
	viper.SetDefault("chat.grpc.client.forwarder.endpoint", "localhost:4002")
	viper.SetDefault("chat.subscriber.id", "rc.msg."+os.Getenv("HOSTNAME"))
	viper.SetDefault("chat.message.maxNum", 5000)
	viper.SetDefault("chat.message.paginationNum", 5000)
	viper.SetDefault("chat.message.maxSizeByte", 4096)
	viper.SetDefault("chat.jwt.secret", "replaceme")
	viper.SetDefault("chat.jwt.expirationSecond", 86400)

	viper.SetDefault("ai.baseURL", "http://localhost:8090")
	viper.SetDefault("ai.requestTimeoutMilli", 30000)

	viper.SetDefault("match.http.server.port", "5002")
	viper.SetDefault("match.http.server.maxConn", 200)
	viper.SetDefault("match.http.server.swag", false)
	viper.SetDefault("match.grpc.client.chat.endpoint", "localhost:4000")
	viper.SetDefault("match.grpc.client.user.endpoint", "localhost:4001")
	viper.SetDefault("match.grpc.client.safety.endpoint", "localhost:4003")
	viper.SetDefault("match.grpc.client.discovery.endpoint", "localhost:4004")

	viper.SetDefault("uploader.http.server.port", "5003")
	viper.SetDefault("uploader.http.server.swag", false)
	viper.SetDefault("uploader.http.server.maxBodyByte", "67108864")   // 64MB
	viper.SetDefault("uploader.http.server.maxMemoryByte", "16777216") // 16MB
	viper.SetDefault("uploader.s3.endpoint", "http://localhost:9000")
	viper.SetDefault("uploader.s3.publicEndpoint", "http://localhost:9000")
	viper.SetDefault("uploader.s3.region", "us-east-1")
	viper.SetDefault("uploader.s3.bucket", "myfilebucket")
	viper.SetDefault("uploader.s3.accessKey", "")
	viper.SetDefault("uploader.s3.secretKey", "")
	viper.SetDefault("uploader.s3.presignLifetimeSecond", 86400)
	viper.SetDefault("uploader.rateLimit.channelUpload.rps", 200)
	viper.SetDefault("uploader.rateLimit.channelUpload.burst", 50)

	viper.SetDefault("user.http.server.port", "5004")
	viper.SetDefault("user.http.server.swag", false)
	viper.SetDefault("user.grpc.server.port", "4001")
	viper.SetDefault("user.oauth.cookie.maxAge", 3600)
	viper.SetDefault("user.oauth.cookie.path", "/")
	viper.SetDefault("user.oauth.cookie.domain", "")
	viper.SetDefault("user.oauth.cookie.secure", false)
	viper.SetDefault("user.oauth.cookie.httpOnly", true)
	viper.SetDefault("user.oauth.cookie.sameSite", "lax")
	viper.SetDefault("user.oauth.google.redirectUrl", "http://localhost/api/user/oauth2/google/callback")
	viper.SetDefault("user.oauth.google.clientId", "")
	viper.SetDefault("user.oauth.google.clientSecret", "")
	viper.SetDefault("user.oauth.google.scopes", "https://www.googleapis.com/auth/userinfo.email,https://www.googleapis.com/auth/userinfo.profile")
	viper.SetDefault("user.auth.cookie.maxAge", 86400)
	viper.SetDefault("user.auth.cookie.path", "/")
	viper.SetDefault("user.auth.cookie.domain", "")
	viper.SetDefault("user.auth.cookie.secure", false)
	viper.SetDefault("user.auth.cookie.httpOnly", true)
	viper.SetDefault("user.auth.cookie.sameSite", "lax")

	viper.SetDefault("forwarder.grpc.server.port", "4002")

	viper.SetDefault("safety.http.server.port", "5005")
	viper.SetDefault("safety.grpc.server.port", "4003")
	viper.SetDefault("safety.warnScore", 40)
	viper.SetDefault("safety.blockScore", 70)
	viper.SetDefault("safety.rateLimit.windowSecond", 10)
	viper.SetDefault("safety.rateLimit.maxMessages", 12)
	viper.SetDefault("safety.rateLimit.muteSecond", 30)
	viper.SetDefault("safety.workers", 4)

	viper.SetDefault("discovery.http.server.port", "5006")
	viper.SetDefault("discovery.grpc.server.port", "4004")
	viper.SetDefault("discovery.grpc.client.safety.endpoint", "localhost:4003")
	viper.SetDefault("discovery.candidateLimit", 20)
	viper.SetDefault("discovery.recentPeerDay", 7)
	viper.SetDefault("discovery.workers", 4)

	viper.SetDefault("workspace.http.server.port", "5007")
	viper.SetDefault("workspace.grpc.server.port", "4005")
	viper.SetDefault("workspace.grpc.client.chat.endpoint", "localhost:4000")
	viper.SetDefault("workspace.workers", 4)
	viper.SetDefault("workspace.reminderPollSecond", 15)

	viper.SetDefault("notification.pollIntervalSecond", 2)
	viper.SetDefault("notification.leaseSecond", 30)
	viper.SetDefault("notification.maxAttempts", 8)
	viper.SetDefault("notification.baseRetrySecond", 1)
	viper.SetDefault("notification.maxRetrySecond", 900)
	viper.SetDefault("notification.batchSize", 50)
	viper.SetDefault("notification.workerID", "")

	viper.SetDefault("kafka.addrs", "localhost:9092")
	viper.SetDefault("kafka.version", "1.0.0")

	viper.SetDefault("cassandra.hosts", "localhost")
	viper.SetDefault("cassandra.port", 9042)
	viper.SetDefault("cassandra.user", "admin")
	viper.SetDefault("cassandra.password", "nexuschat165")
	viper.SetDefault("cassandra.keyspace", "NexusChat")

	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.addrs", "localhost:6379")
	viper.SetDefault("redis.expirationHour", 24)
	viper.SetDefault("redis.minIdleConn", 16)
	viper.SetDefault("redis.poolSize", 64)
	viper.SetDefault("redis.readTimeoutMilliSecond", 3000)
	viper.SetDefault("redis.writeTimeoutMilliSecond", 3000)

	viper.SetDefault("observability.prometheus.port", "8080")
	viper.SetDefault("observability.tracing.otlpEndpoint", "")
}

func NewConfig() (*Config, error) {
	setDefault()

	var c Config
	if err := viper.Unmarshal(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
