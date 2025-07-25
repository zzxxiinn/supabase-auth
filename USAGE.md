## SMS Config
```
# 短信提供商选择
SMS_PROVIDER=aliyun

# 短信模板代码
SMS_TEMPLATE=SMS_154950909
```

## Aliyun
```
# 阿里云 Access Key ID
SMS_ALIYUN_ACCESS_KEY_ID=your_access_key_id

# 阿里云 Access Key Secret  
SMS_ALIYUN_ACCESS_KEY_SECRET=your_access_key_secret

# 阿里云短信服务终端
SMS_ALIYUN_ENDPOINT=https://dysmsapi.aliyuncs.com

# 阿里云短信签名（支持中文）
SMS_ALIYUN_SIGN_NAME=您的短信签名

# 阿里云短信扩展码（可选）
SMS_ALIYUN_SMS_UP_EXTEND_CODE=
```

## Gotrue docker file
```
# 在 docker-compose.yml 中映射到 GoTrue 容器
GOTRUE_SMS_ALIYUN_ACCESS_KEY_ID=${SMS_ALIYUN_ACCESS_KEY_ID}
GOTRUE_SMS_ALIYUN_ACCESS_KEY_SECRET=${SMS_ALIYUN_ACCESS_KEY_SECRET}
GOTRUE_SMS_ALIYUN_ENDPOINT=${SMS_ALIYUN_ENDPOINT}
GOTRUE_SMS_ALIYUN_SIGN_NAME=${SMS_ALIYUN_SIGN_NAME}
GOTRUE_SMS_ALIYUN_SMS_UP_EXTEND_CODE=${SMS_ALIYUN_SMS_UP_EXTEND_CODE}
```

[see](https://github.com/estel-li/supabase_auth_aliyun_wechat)