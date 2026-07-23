from typing import Annotated

from fastapi import Depends, Request

from app.config.settings import Settings


def get_request_settings(request: Request) -> Settings:
    return request.app.state.settings


SettingsDep = Annotated[Settings, Depends(get_request_settings)]
