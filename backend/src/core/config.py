from pydantic import field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    # App
    APP_NAME: str = "FastAPI Template"
    APP_VERSION: str = "0.1.0"
    MODE: str = "dev"

    # Database
    DATABASE_URL: str

    # Auth
    SECRET_KEY: str
    ALGORITHM: str = "HS256"
    ACCESS_TOKEN_EXPIRE_MINUTES: int = 30
    REFRESH_TOKEN_EXPIRE_DAYS: int = 7

    # Database Pool
    DB_POOL_SIZE: int = 20
    DB_MAX_OVERFLOW: int = 10
    DB_POOL_RECYCLE: int = 300

    # CORS
    CORS_ORIGINS: list[str] = ["http://localhost:3000"]

    @field_validator("SECRET_KEY")
    @classmethod
    def validate_secret_key(cls, v: str) -> str:
        if len(v) < 32:
            raise ValueError("SECRET_KEY must be at least 32 characters for HS256")
        return v

    @model_validator(mode="after")
    def reject_placeholder_secret_in_prod(self) -> "Settings":
        # Checks the parsed MODE field, so it works whether MODE comes from
        # .env or the OS environment (os.getenv would miss env_file-only values).
        if self.MODE == "prod" and "change-me" in self.SECRET_KEY:
            raise ValueError(
                "SECRET_KEY cannot contain default placeholder 'change-me' in production mode!"
            )
        return self

    model_config = SettingsConfigDict(env_file=".env")

    @property
    def async_database_url(self) -> str:
        """Convert DATABASE_URL to async version (postgresql+asyncpg://)."""
        url = self.DATABASE_URL
        if url.startswith("postgresql://"):
            return url.replace("postgresql://", "postgresql+asyncpg://", 1)
        if url.startswith("postgres://"):
            return url.replace("postgres://", "postgresql+asyncpg://", 1)
        return url


settings = Settings()
