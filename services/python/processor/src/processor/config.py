from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    service_name: str = "pociag.processor"
    database_dsn: str = ""
    otlp_exporter_endpoint: str | None = None
    log_level: str = "INFO"
    s3_endpoint: str = ""
    s3_bucket: str = "pociag-lake"
    s3_access_key: str = ""
    s3_secret_key: str = ""
