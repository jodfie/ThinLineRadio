#!/usr/bin/env python3
"""
Bridge icad_tone_detection results to TLR Tone JSON for auto-learn comparison tests.

Usage:
  python3 icad_tone_bridge.py /path/to/audio.mp3
  python3 icad_tone_bridge.py --json /path/to/audio.mp3   # compact JSON only

Requires: pip install icad_tone_detection  (and ffmpeg in PATH)
"""
from __future__ import annotations

import json
import os
import sys
import warnings

os.environ.setdefault("PYTHONWARNINGS", "ignore")
warnings.filterwarnings("ignore")

from icad_tone_detection import tone_detect  # noqa: E402


def icad_result_to_tones(result) -> list[dict]:
    tones: list[dict] = []

    for hit in result.two_tone_result or []:
        detected = hit.get("detected") or []
        if len(detected) < 2:
            continue
        a_hz = float(detected[0])
        b_hz = float(detected[1])
        start = float(hit.get("start", 0))
        end = float(hit.get("end", start))
        a_len = float(hit.get("tone_a_length", 0))
        b_len = float(hit.get("tone_b_length", 0))
        if a_len <= 0:
            a_len = max(0.0, (end - start) * 0.3)
        if b_len <= 0:
            b_len = max(0.0, end - start - a_len)
        a_end = start + a_len
        b_start = max(a_end, end - b_len)
        tones.append(
            {
                "frequency": a_hz,
                "startTime": round(start, 3),
                "endTime": round(a_end, 3),
                "duration": round(a_len, 3),
                "toneType": "A",
            }
        )
        tones.append(
            {
                "frequency": b_hz,
                "startTime": round(b_start, 3),
                "endTime": round(end, 3),
                "duration": round(b_len, 3),
                "toneType": "B",
            }
        )

    for hit in result.long_result or []:
        freq = hit.get("detected")
        if isinstance(freq, (list, tuple)):
            freq = freq[0] if freq else 0
        freq = float(freq or 0)
        if freq <= 0:
            continue
        start = float(hit.get("start", 0))
        end = float(hit.get("end", start))
        length = float(hit.get("length", end - start))
        if length <= 0:
            length = end - start
        tones.append(
            {
                "frequency": freq,
                "startTime": round(start, 3),
                "endTime": round(end, 3),
                "duration": round(length, 3),
                "toneType": "Long",
            }
        )

    tones.sort(key=lambda t: t["startTime"])
    return tones


def main() -> int:
    warnings.filterwarnings("ignore")
    args = sys.argv[1:]
    json_only = False
    if args and args[0] == "--json":
        json_only = True
        args = args[1:]
    if len(args) != 1:
        print("usage: icad_tone_bridge.py [--json] <audio-file>", file=sys.stderr)
        return 2

    path = args[0]
    result = tone_detect(path, debug=False)
    payload = {
        "path": path,
        "two_tone": result.two_tone_result or [],
        "long": result.long_result or [],
        "pulsed": result.pulsed_result or [],
        "tones": icad_result_to_tones(result),
    }
    if json_only:
        print(json.dumps(payload))
    else:
        print(json.dumps(payload, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
