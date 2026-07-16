import hashlib
import time

import requests

DEFAULT_BASE_URL = "https://www.foxesscloud.com"


class FoxessApiError(Exception):
    pass


class FoxessClient:
    """Cliente para a FoxESS Cloud OpenAPI usando autenticação por token privado
    (header token/timestamp/signature) — não usa OAuth nem client-id."""

    def __init__(self, api_key: str, base_url: str = DEFAULT_BASE_URL):
        self.api_key = api_key
        self.base_url = base_url

    def _headers(self, path: str) -> dict:
        timestamp = str(round(time.time() * 1000))
        # A FoxESS valida a assinatura sobre a sequência literal de 4 caracteres
        # "\r\n" (barra-invertida, r, barra-invertida, n), não sobre bytes CR+LF
        # reais, apesar do que a doc sugere — confirmado testando contra a API real.
        signature = hashlib.md5(
            f"{path}\\r\\n{self.api_key}\\r\\n{timestamp}".encode("utf-8")
        ).hexdigest()
        return {
            "token": self.api_key,
            "timestamp": timestamp,
            "signature": signature,
            "lang": "en",
            "Content-Type": "application/json",
        }

    def _request(self, method: str, path: str, body: dict | None = None) -> object:
        headers = self._headers(path)
        if method == "GET":
            resp = requests.get(f"{self.base_url}{path}", headers=headers, params=body, timeout=15)
        else:
            resp = requests.post(f"{self.base_url}{path}", headers=headers, json=body, timeout=15)
        resp.raise_for_status()
        data = resp.json()
        if data.get("errno") != 0:
            raise FoxessApiError(f"{path} falhou: {data}")
        return data["result"]

    def get_device_list(self) -> list[dict]:
        result = self._request("POST", "/op/v0/device/list", {"currentPage": 1, "pageSize": 10})
        return result["data"]

    def get_real_query(self, sn: str, variables: list[str]) -> list[dict]:
        return self._request("POST", "/op/v0/device/real/query", {"sn": sn, "variables": variables})

    def get_history_query(self, sn: str, variables: list[str], begin_ms: int, end_ms: int) -> list[dict]:
        return self._request(
            "POST",
            "/op/v0/device/history/query",
            {"sn": sn, "variables": variables, "begin": begin_ms, "end": end_ms},
        )
