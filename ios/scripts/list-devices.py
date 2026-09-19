"""繋がっている iOS デバイスを「名前 <TAB> デベロッパモードの状態」で出す。

devicectl の JSON から拾うだけ。device.sh から呼ばれる。
"""

import json
import sys


def main() -> None:
    try:
        with open(sys.argv[1], encoding="utf-8") as f:
            devices = json.load(f)["result"]["devices"]
    except (OSError, ValueError, KeyError, IndexError):
        # デバイスが1台も無いと devicectl が JSON を書かないことがある。
        # 呼ぶ側が「見つからない」として扱うので、黙って何も出さない
        return

    for d in devices:
        if d.get("hardwareProperties", {}).get("platform") != "iOS":
            continue
        if d.get("connectionProperties", {}).get("pairingState") != "paired":
            continue

        props = d.get("deviceProperties", {})
        name = props.get("name") or ""
        dev_mode = props.get("developerModeStatus") or "unknown"
        print(f"{name}\t{dev_mode}")


if __name__ == "__main__":
    main()
