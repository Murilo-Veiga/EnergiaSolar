import time

import requests

DEFAULT_BASE_URL = "https://la5.fusionsolar.huawei.com"

# Restrição conhecida do Northbound API: uma mesma interface não pode ser
# chamada de novo antes de ~5 minutos (senão retorna failCode 407
# ACCESS_FREQUENCY_IS_TOO_HIGH). Login não é afetado por essa restrição.
MIN_INTERFACE_INTERVAL_SECONDS = 300


class HuaweiNbiError(Exception):
    pass


class HuaweiClient:
    """Cliente para a Northbound Interface (NBI) oficial do FusionSolar."""

    def __init__(self, username: str, system_code: str, base_url: str = DEFAULT_BASE_URL):
        self.username = username
        self.system_code = system_code
        self.base_url = base_url
        self.session = requests.Session()
        self.xsrf_token = None
        self._last_call_at = {}

    def login(self):
        resp = self.session.post(
            f"{self.base_url}/thirdData/login",
            json={"userName": self.username, "systemCode": self.system_code},
            timeout=15,
        )
        resp.raise_for_status()
        data = resp.json()
        if not data.get("success"):
            raise HuaweiNbiError(f"login falhou: {data}")
        self.xsrf_token = resp.headers["xsrf-token"]

    def _post(self, path: str, body: dict) -> object:
        last_call = self._last_call_at.get(path)
        if last_call is not None:
            wait = MIN_INTERFACE_INTERVAL_SECONDS - (time.monotonic() - last_call)
            if wait > 0:
                time.sleep(wait)
        self._last_call_at[path] = time.monotonic()

        resp = self.session.post(
            f"{self.base_url}{path}",
            headers={"XSRF-TOKEN": self.xsrf_token},
            json=body,
            timeout=15,
        )
        resp.raise_for_status()
        data = resp.json()
        if not data.get("success"):
            raise HuaweiNbiError(f"{path} falhou: {data}")
        return data["data"]

    def get_station_list(self) -> list[dict]:
        return self._post("/thirdData/getStationList", {})

    def get_dev_list(self, station_codes: str) -> list[dict]:
        return self._post("/thirdData/getDevList", {"stationCodes": station_codes})

    def get_station_real_kpi(self, station_codes: str) -> list[dict]:
        return self._post("/thirdData/getStationRealKpi", {"stationCodes": station_codes})

    def get_dev_real_kpi(self, dev_ids: str, dev_type_id: int) -> list[dict]:
        return self._post("/thirdData/getDevRealKpi", {"devIds": dev_ids, "devTypeId": dev_type_id})

    def get_alarm_list(self, station_codes: str) -> list[dict]:
        return self._post("/thirdData/getAlarmList", {"stationCodes": station_codes})
