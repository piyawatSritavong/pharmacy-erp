#!/usr/bin/env python3
"""Extract the four Ocha product/category MHTML snapshots into seed JSON.

The source pages do not expose an Ocha SKU or barcode.  This extractor keeps
every source row as provenance, creates deterministic master identifiers, and
only merges names that differ by Unicode presentation, whitespace, or
punctuation.  It intentionally does not perform fuzzy-name merging.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import mimetypes
import re
import statistics
import unicodedata
import uuid
from collections import Counter, defaultdict
from dataclasses import dataclass
from datetime import date, datetime, timedelta, timezone
from email import policy
from email.parser import BytesParser
from html.parser import HTMLParser
from pathlib import Path
from typing import Any, Iterable
from urllib.request import Request, urlopen


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_OUTPUT = REPOSITORY_ROOT / "backend/internal/app/seeddata/ocha_catalog.json"
GENERATED_ON = date(2026, 8, 5)
RANDOM_SEED = "pharmacy-erp-ocha-catalog-v1"
UUID_NAMESPACE = uuid.UUID("8e14c470-f79f-4b57-b8fd-4c4552f82f33")
maxProductImageBytes = 5 << 20


@dataclass(frozen=True)
class SourceSpec:
    code: str
    name: str
    branch_type: str
    product_file: str
    category_file: str
    expected_products: int
    expected_categories: int


SOURCE_SPECS = (
    SourceSpec(
        "PHH",
        "หน้ารพ.พหลฯ",
        "branch",
        "reference-branch/อุปกรณ์การแพทย์ สาขาหน้ารพ.พหลฯ/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ18.mhtml",
        "reference-branch/อุปกรณ์การแพทย์ สาขาหน้ารพ.พหลฯ/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ19.mhtml",
        431,
        12,
    ),
    SourceSpec(
        "PHS",
        "หน้าตลาดผาสุก",
        "branch",
        "reference-branch/อุปกรณ์การแพทย์ สาขาตลาดผาสุก/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ3-1.mhtml",
        "reference-branch/อุปกรณ์การแพทย์ สาขาตลาดผาสุก/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ3-2.mhtml",
        429,
        12,
    ),
    SourceSpec(
        "NPT",
        "จังหวัดนครปฐม",
        "branch",
        "reference-branch/อุปกรณ์การแพทย์ สาขาจ.นครปฐม/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ2-13.mhtml",
        "reference-branch/อุปกรณ์การแพทย์ สาขาจ.นครปฐม/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ2-14.mhtml",
        298,
        12,
    ),
    SourceSpec(
        "MES",
        "MES",
        "branch",
        "reference-branch/MES อุปกรณ์การแพทย์ เตียงพยาบาล อุปกรณ์ดูแลผู้ป่วย รถเข็น ไม้เท้าครบวงจร/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ1-12.mhtml",
        "reference-branch/MES อุปกรณ์การแพทย์ เตียงพยาบาล อุปกรณ์ดูแลผู้ป่วย รถเข็น ไม้เท้าครบวงจร/Ocha POS - แอปพลิเคชันบริหารจัดการธุรกิจ1-13.mhtml",
        202,
        8,
    ),
)

BRANCH_PRIORITY = {"MES": 0, "PHH": 1, "PHS": 2, "NPT": 3}
CATEGORY_COLORS = (
    "#D71920",
    "#F7B32B",
    "#1D4ED8",
    "#059669",
    "#7C3AED",
    "#DB2777",
    "#0891B2",
    "#EA580C",
    "#4F46E5",
    "#65A30D",
    "#9333EA",
    "#475569",
)

PRICE_RANGES = {
    "3M FUTURO": (100, 2500),
    "HEALTHY CARE": (100, 2500),
    "POP SUPPORT": (100, 2500),
    "ถังออกซิเจน": (1500, 15000),
    "นม/อาหารทางการแพทย์/อาหารผู้ป่วย": (200, 2500),
    "รถเข็น/WHEELCHAIR": (1500, 45000),
    "เครื่องชั่งน้ำหนัก/เครื่องวัดไข้": (200, 5000),
    "เครื่องผลิตอ๊อกซิเจน": (5000, 60000),
    "เครื่องวัดความดัน/เจาะน้ำตาล/พ่นยา/ดูดเสมหะ": (200, 12000),
    "เตียงผู้ป่วย/ที่นอนลม/เบาะ/เสาน้ำเกลือ/ตู้หัวเตียง": (500, 90000),
    "เบ็ดเตล็ด/อื่นๆ": (20, 5000),
    "ไม้เท้า/ไม้ค้ำ/พยุงเดิน/ROLLATOR/เก้าอี้นั่งถ่าย": (150, 15000),
    "": (100, 10000),
}

# These resolutions were reviewed against the source names, prices and embedded
# product photos.  Keeping them explicit makes later re-generation auditable and
# prevents a broad fuzzy matcher from silently combining different models.
MERGE_GROUPS: tuple[tuple[str, str, tuple[str, ...]], ...] = (
    ("abn-ecolite-manual", "ABN ECOLITE เครื่องวัดความดันแบบแมนนวล", ("ABN ECOLITE เครื่องวัดความดัน (MANUAL)", "ABN ECOLITE เครื่องวัดความดันแบบแมนนัวล์")),
    ("bedpan-b02", "หม้อนอนพลาสติก B-02 (PLASTIC BEDPAN)", ("B-02หม้อนอนPLASTIC BEDPAN (พลาสติก)", "หม้อนอนPLASTIC BEDPAN (พลาสติก)", "หม้อนอนPLASTIC BEDPAN (พลาสติก) B-02")),
    ("shower-chair-czb01-blue", "เก้าอี้อาบน้ำรหัส CZB-01 สีฟ้า", ("CBZ-01เก้าอี้อาบน้ำ (ฟ้า)", "เก้าอี้นั่งอาบน้ำCZB01(ฟ้า)", "เก้าอี้อาบน้ำCZB-01(ฟ้า)")),
    ("bed-e02", "เตียงไฟฟ้า E-02 2 ฟังก์ชัน ราวสไลด์ (มือหมุนสำรอง)", ("E-02เตียงไฟฟ้า 2 ฟังก์ชั่น", "E-02เตียงไฟฟ้า 2F ราวสไลด์ (มือหมุน)", "E-02เตียงไฟฟ้า2Fราวสไลด์(มือหมุน)")),
    ("bed-eb2-03", "เตียงไฟฟ้า EB2-03 3 ฟังก์ชัน ปีกนกคู่", ("EB2-03เตียงปีกนกไฟฟ้า(3ฟังก์ชั่น)", "EB2-03เตียงไฟฟ้า 3 ฟังก์ชั่น(ปีกนก)")),
    ("bed-en05", "เตียงพยาบาลไฟฟ้า EN-05 5 ฟังก์ชัน ราวสไลด์", ("EN-05เตียงพยาบาลไฟฟ้า 5f", "EN-05เตียงพยาบาลไฟฟ้า5Fราวสไลด์")),
    ("futuro-elbow-adjustable", "3M FUTURO พยุงข้อศอกแบบปรับได้ Sport Support", ("F-ADJ.พยุงข้อศอก ปรับได้", "F-ADJ.พยุงข้อศอก ปรับได้ Sport support")),
    ("cane-fs931", "ไม้เท้าก้านร่ม 4 ขา อะลูมิเนียม FS931", ("FS931ไม้เท้าก้านร่ม 4ขา อะลูมิเนียม", "ไม้เท้าก้านร่ม 4ขา อะลูมิเนียม รุ่น 931")),
    ("wheelchair-hospro-green-small", "รถเข็น HOSPRO อะลูมิเนียม สีเขียว ล้อเล็ก", ("HOSPRO อัลลอย์ เขียว ล้อเล็ก", "HOSPRO เขียว ล้อเล็ก อลู")),
    ("karma-walker-gray", "KARMA Walker สีเทา", ("KARMA walker", "KARMA walker เทา")),
    ("bed-m03", "เตียงมือหมุน M-03 3 ไกร์ ราวสไลด์", ("M-03เตียง3ไกร์(ใหม่) นำเข้า", "M-03เตียง3ไกร์มือหมุน ราวสไลด์", "M-03เตียงมือหมุน3ไกร์")),
    ("bed-mb1-03", "เตียงมือหมุน MB1-03 3 ไกร์ ปีกนกเดี่ยว", ("MB1-03เตียง 3 ไกร์ ปีกนกเดี่ยว", "MB1-03เตียง 3 ไกร์ มือหมุนปีกนกเดี่ยว", "MB1-03เตียงมือหมุน3ไกร์ (ปีกนกเดี่ยว)")),
    ("bed-mw03", "เตียงมือหมุน MW-03 3 ไกร์ ปีกไม้", ("MW-03 เตียง3ไกร์(ไม้)", "MW-03เตียงมือหมุน3ไกร์ (ปีกไม้)")),
    ("oxygen-nebulizer-adult", "หน้ากากออกซิเจนสำหรับพ่นยา ผู้ใหญ่", ("OXYGEN MASK ผู้ใหญ่", "OXYGEN MASK+พ่นยา", "OXYGEN MASK+พ่นยา ผู้ใหญ่")),
    ("oxygen-nebulizer-child", "หน้ากากออกซิเจนสำหรับพ่นยา เด็ก", ("OXYGEN MASK เด็ก", "OXYGEN MASK+พ่นยา เด็ก")),
    ("oximeter-jumper-500d", "เครื่องวัดออกซิเจนปลายนิ้ว JUMPER JPD-500D", ("Oximeter jumper", "Oximeter jumper 500D")),
    ("oximeter-yuwell-yx302", "เครื่องวัดออกซิเจนปลายนิ้ว YUWELL YX302", ("Oximeter yuwell (Fingertip plus) YX302", "Oximeter yuwell (Fingerttip plus) YX302")),
    ("rollator-ca01-blue", "รถช่วยเดิน Rollator-H CA-01 สีน้ำเงิน 4 ล้อ", ("Rollator CA-01 (น้ำเงิน) 4ล้อ", "Rollator(น้ำเงิน)MES-01", "Rollator-H (CA-01)", "Rollator-H (น้ำเงิน)")),
    ("rollator-orange-five", "รถช่วยเดิน Rollator สีส้ม 5 ล้อ", ("Rollator(ส้ม)5ล้อ", "Rollator(ส้ม/แดง)5ล้อ")),
    ("tracheostomy-oxygen-humidifier", "ชุดให้ออกซิเจนสำหรับผู้เจาะคอ พร้อมกระบอกทำความชื้น (งวงช้าง)", ("SET งวงช้าง (กระบอกทำความชื้น)", "SET เจาะคองวงช้าง")),
    ("bp-sinocare-aesu111", "เครื่องวัดความดัน SINOCARE AES-U111", ("Sinocareเครื่องวัดความดัน AES-U111", "เครื่องวัดความดัน SINOCARE AES-U111")),
    ("suction-yuwell-7eh1", "เครื่องดูดเสมหะ YUWELL 7E-H1", ("Yuwell(7E-H1)ดูดเสมหะ", "Yuwell(7E-H1)เครื่องดูดเสมหะ")),
    ("male-urinal", "กระบอกปัสสาวะชาย", ("กระบอกปัสสาวะ", "กระบอกปัสสาวะชาย")),
    ("air-mattress-tc-medicare", "ที่นอนลมรังผึ้ง TC Medicare", ("ที่นอนลมรังผึ้ง TC medicare", "ที่นอนลมรังผึ้ง medicare")),
    ("pvc-draw-sheet-blue", "แผ่นยางปูกันเปื้อน PVC 150×90 ซม. สีน้ำเงิน", ("ผ้ายางปูกันเปื้อนPVC 150x90cm (สีน้ำเงิน)", "แผ่นยางปูกันเปื้อนPVC 150x90cm (สีน้ำเงิน)")),
    ("wheelchair-yuwell-h053c", "รถเข็น YUWELL H-053C", ("รถเข็นYUWELL รุ่นH-053C", "รถเข็นYuwell H-053C")),
    ("wheelchair-karma-ergo-nimble", "รถเข็นไฟฟ้า KARMA Ergo Nimble", ("รถเข็นไฟฟ้าKarma รุ่น Ergo Nimble", "รถเข็นไฟฟ้าKrama รุ่น Ereo Nimble")),
    ("rubber-bulbs", "ลูกยางต่างๆ", ("ลูกยาง", "ลูกยางต่างๆ")),
    ("commode-fs896", "เก้าอี้นั่งถ่าย FS896", ("เก้านั่งถ่าย(FS896)", "เก้าอี้นั่งถ่าย(FS896)")),
    ("commode-fs696", "เก้าอี้นั่งถ่ายมีพนักพิงและล้อ FS696", ("เก้าอี้นั่งถ่าย FS696 มีล้อ", "เก้าอี้นั่งถ่าย มีพนักพิง มีล้อ FS696", "เก้าอี้นั่งถ่าย มีพนักพิง มีล้อFS696")),
    ("commode-fs894l", "เก้าอี้นั่งถ่ายมีพนักพิง ไม่มีล้อ FS894L", ("เก้าอี้นั่งถ่าย FS894L ไม่มีล้อ", "เก้าอี้นั่งถ่าย มีพนักพิง ไม่มีล้อ FS894L", "เก้าอี้นั่งถ่ายFS894L (มีพนักพิง ไม่มีล้อ)")),
    ("shower-chair-czb11", "เก้าอี้อาบน้ำ CZB11", ("เก้าอี้นั่งอาบน้ำCZB11", "เก้าอี้อาบน้ำCZB11")),
    ("oxygen-flowmeter-regulator", "ชุดเกจ์ออกซิเจน (Oxygen Flowmeter Regulator)", ("เซตเกจ์ อ๊อกซิเจน", "เซตเกร์")),
    ("blendera-mf-2-5kg", "อาหารทางการแพทย์ BLENDERA-MF 2.5 กก.", ("เบลนเดอร่า เอ็มเอฟ 2.5kg.", "เบลนเดอล่า BLENDERA-MF 2.5kg.")),
    ("overbed-table-cbz03", "โต๊ะคร่อมเตียงไม้ CBZ03 รุ่นแรก", ("โต๊ะ(ไม้)คร่อมเตียง", "โต๊ะคร่อมเตียง(CBZ03)รุ่นแรก")),
    ("oxygen-filter-yuwell-8f", "ไส้กรองเครื่องผลิตออกซิเจน YUWELL 8F-3AW/8F-5AW", ("ไส้กรองเครื่องผลิต8F3AW of 8F5AW", "ไส้กรองเครื่องผลิต8F3AW และ8F5AW")),
)

for _model in ("1", "10", "11", "3", "4", "5", "7", "8", "9"):
    MERGE_GROUPS += ((f"mcl-{_model}", f"MCL-{_model}", (f"CL-{_model}", f"MCL-{_model}")),)

VARIANT_GROUPS = (
    ("arm-sling", "ARM SLING", ("Size 1", "Size 2", "Size 3", "Size 4"), ("ARM SLING", "ผ้าคล้องแขน Arm Sling")),
    ("futuro-ankle-comfort", "3M FUTURO พยุงข้อเท้า Ankle Comfort Support", ("Size S", "Size M", "Size L"), ("F-พยุงข้อเท้า ชนิดสวม ANKLE COMFORT SUPPORT", "F-พยุงข้อเท้าชนิดสวม COMFORTSUPPORT")),
    ("oxygen-mask-bag", "หน้ากากออกซิเจนพร้อมถุงสำรอง", ("ผู้ใหญ่", "เด็ก"), ("OXYGEN MASK WITH BAG", "OXYGEN MASK WITH BAG ผู้ใหญ่")),
    ("vr-wrist-beige", "VR อุปกรณ์พยุงข้อมือล็อกฝ่ามือ สีเนื้อ", ("Size S", "Size M", "Size L", "Size XL"), ("VR-พยุงข้อมือ ล๊อคฝ่ามือ สีเนื้อ", "VR-พยุงข้อมือ ล๊อคฝ่ามือ สีเนื้อ S")),
    ("wellness-pants-10", "WELLNESS กางเกงผ้าอ้อมผู้ใหญ่", ("Size M (10 ชิ้น)", "Size L (10 ชิ้น)"), ("WELLNESS กางเกงผ้าอ้อมผู้ใหญ่", "WELLNESS กางเกงผ้าอ้อมผู้ใหญ่(M)")),
    ("wellness-tape-40", "WELLNESS ผ้าอ้อมผู้ใหญ่แบบเทป", ("Size M (40 ชิ้น)", "Size L (40 ชิ้น)"), ("WELLNESS เปลือยเทปกาวผ้าอ้อมผู้ใหญ่ (40 ชิ้น)", "WELLNESS เปลือยเทปกาวผ้าอ้อมผู้ใหญ่ M (40 ชิ้น)")),
    ("hot-water-bag", "กระเป๋าน้ำร้อน", ("Size S", "Size L"), ("กระเป๋าน้ำร้อน", "กระเป๋าน้ำร้อน S", "กระเป๋าน้ำร้อน L")),
    ("stainless-measure", "ถ้วยตวงสแตนเลสทรงกระบอก มีสเกลด้านใน", ("500 ml", "1,000 ml", "1,500 ml", "2,000 ml", "3,000 ml"), ("ถ้วยตวงสแตนเลสทรงกระบอก มีสเกลด้านใน", "ถ้วยตวงสแตนเลสทรงกระบอก มีสเกลด้านใน (เหยือก)")),
)

SEPARATE_NAME_OVERRIDES = {
    "OMRON รุ่น HBF222T": "เครื่องชั่งวิเคราะห์องค์ประกอบร่างกาย OMRON HBF-222T",
    "เครื่องชั่งอัจฉริยะ SMART SCALE PW-100": "เครื่องชั่งอัจฉริยะ SMART SCALE PW-100",
    "SOMA150.5(ล้อเล็ก)ดำ": "รถเข็น SOMA 150.5 ล้อเล็ก สีดำ",
    "SOMA150.5(ล้อเล็ก)เทา": "รถเข็น SOMA 150.5 ล้อเล็ก สีเทา",
    "SOMA150.5(ล้อใหญ่)ดำ": "รถเข็น SOMA 150.5 ล้อใหญ่ สีดำ",
    "SOMA150.5(ล้อใหญ่)เทา": "รถเข็น SOMA 150.5 ล้อใหญ่ สีเทา",
    "เก้าอี้นั่งถ่ายCA616L": "เก้าอี้นั่งถ่าย CA616L",
    "แอลกอฮอล์": "แอลกอฮอล์ชนิดน้ำ",
    "แอลกอฮอร์ก้อน": "แอลกอฮอล์ก้อน",
}

SKIP_ALL_IMAGE_NAMES = {
    "เครื่องชั่งอัจฉริยะ SMART SCALE PW-100",
    "แอลกอฮอล์ก้อน",
}

SKIP_IMAGE_BRANCHES = {("เก้าอี้นั่งถ่าย CA616L", "NPT")}


class OchaTableParser(HTMLParser):
    """Collects div-based table rows used by the saved Ocha React pages."""

    def __init__(self, actionable_only: bool) -> None:
        super().__init__(convert_charrefs=True)
        self.actionable_only = actionable_only
        self.rows: list[dict[str, Any]] = []
        self._inside_row = False
        self._row_depth = 0
        self._cells: list[str] = []
        self._cell_buffer: list[str] | None = None
        self._image_url = ""

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        attr = {key: value or "" for key, value in attrs}
        classes = set(attr.get("class", "").split())
        is_row = tag == "div" and "tr" in classes
        if self.actionable_only:
            is_row = is_row and "actionable" in classes
        if is_row and not self._inside_row:
            self._inside_row = True
            self._row_depth = 1
            self._cells = []
            self._cell_buffer = None
            self._image_url = ""
            return
        if not self._inside_row:
            return
        if tag == "div":
            self._row_depth += 1
            if self._row_depth == 2 and ("td" in classes or "th" in classes):
                self._cell_buffer = []
        elif tag == "img" and not self._image_url:
            self._image_url = attr.get("src", "")

    def handle_endtag(self, tag: str) -> None:
        if not self._inside_row or tag != "div":
            return
        if self._row_depth == 2 and self._cell_buffer is not None:
            self._cells.append(collapse_whitespace("".join(self._cell_buffer)))
            self._cell_buffer = None
        self._row_depth -= 1
        if self._row_depth == 0:
            self.rows.append({"cells": self._cells, "image_url": self._image_url})
            self._inside_row = False

    def handle_data(self, data: str) -> None:
        if self._cell_buffer is not None:
            self._cell_buffer.append(data)


def collapse_whitespace(value: str) -> str:
    return " ".join(value.split())


def canonical_signature(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value).casefold()
    return re.sub(r"[^\wก-๙]+", "", normalized)


def normalize_category(value: str) -> str:
    # Keep display values in NFC. NFKC decomposes Thai SARA AM, which is useful
    # for matching signatures but should not leak into persisted category names.
    value = collapse_whitespace(unicodedata.normalize("NFC", value))
    if value == "-":
        return ""
    if value in {"ถังอ๊อกจิเจน", "ถังอ๊อกซิเจน", "ถังออกซิเจน"}:
        return "ถังออกซิเจน"
    return value


def stable_uuid(kind: str, key: str) -> str:
    return str(uuid.uuid5(UUID_NAMESPACE, f"{kind}:{key}"))


def stable_int(key: str, minimum: int, maximum: int) -> int:
    if maximum < minimum:
        raise ValueError(f"invalid stable range {minimum}..{maximum}")
    digest = hashlib.sha256(f"{RANDOM_SEED}:{key}".encode("utf-8")).digest()
    return minimum + int.from_bytes(digest[:8], "big") % (maximum - minimum + 1)


def round_money(value: float) -> float:
    return round(value + 1e-9, 2)


def parse_price(label: str) -> float | None:
    match = re.fullmatch(r"฿([\d,]+(?:\.\d+)?)", label.strip())
    if not match:
        return None
    return float(match.group(1).replace(",", ""))


def ean13_for(key: str, used: set[str]) -> str:
    # Prefix 29 is reserved for internal/restricted distribution usage.
    base_number = int.from_bytes(hashlib.sha256(f"ean:{key}".encode()).digest()[:8], "big") % 10_000_000_000
    for offset in range(10_000):
        base = f"29{(base_number + offset) % 10_000_000_000:010d}"
        checksum = (10 - sum((1 if index % 2 == 0 else 3) * int(char) for index, char in enumerate(base)) % 10) % 10
        barcode = base + str(checksum)
        if barcode not in used:
            used.add(barcode)
            return barcode
    raise ValueError("unable to allocate unique EAN-13")


def mhtml_html(path: Path) -> tuple[str, str]:
    with path.open("rb") as handle:
        message = BytesParser(policy=policy.default).parse(handle)
    part = next((item for item in message.walk() if item.get_content_type() == "text/html"), None)
    if part is None:
        raise ValueError(f"no HTML MIME part in {path}")
    payload = part.get_payload(decode=True) or b""
    return payload.decode(part.get_content_charset() or "utf-8", errors="replace"), part.get("Content-Location", "")


def extract_products(path: Path, branch_code: str) -> list[dict[str, Any]]:
    html, page_url = mhtml_html(path)
    parser = OchaTableParser(actionable_only=True)
    parser.feed(html)
    products = []
    for row_index, row in enumerate(parser.rows, start=1):
        cells = row["cells"]
        if len(cells) < 5:
            continue
        products.append(
            {
                "branch_code": branch_code,
                "row_index": row_index,
                "name": cells[2],
                "category": cells[3],
                "price_label": cells[4],
                "image_url": row["image_url"],
                "page_url": page_url,
            }
        )
    return products


def extract_categories(path: Path) -> list[dict[str, Any]]:
    html, _ = mhtml_html(path)
    parser = OchaTableParser(actionable_only=False)
    parser.feed(html)
    categories = []
    for row in parser.rows:
        cells = row["cells"]
        if len(cells) != 5 or not re.fullmatch(r"\d+\s*สินค้า\(s\)", cells[2]):
            continue
        categories.append(
            {
                "name": cells[1],
                "source_product_count": int(re.search(r"\d+", cells[2]).group()),
                "source_sort_order": int(cells[3]),
            }
        )
    return categories


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def choose_name(records: list[dict[str, Any]]) -> str:
    counts = Counter(record["name"] for record in records)
    first_priority: dict[str, int] = {}
    for record in records:
        first_priority[record["name"]] = min(
            first_priority.get(record["name"], 99), BRANCH_PRIORITY[record["branch_code"]]
        )
    return min(counts, key=lambda name: (-counts[name], first_priority[name], name.casefold()))


def choose_category(key: str, name: str, records: list[dict[str, Any]]) -> str:
    if key.endswith(":walking"):
        return "ไม้เท้า/ไม้ค้ำ/พยุงเดิน/ROLLATOR/เก้าอี้นั่งถ่าย"
    categories = [normalize_category(record["category"]) for record in records]
    categories = [category for category in categories if category]
    if not categories:
        return ""
    if "เครื่องช่วยฟัง" in name:
        return "เบ็ดเตล็ด/อื่นๆ"
    if "เครื่องวัดอุณหภูมิอินฟราเรด" in name:
        return "เครื่องชั่งน้ำหนัก/เครื่องวัดไข้"
    counts = Counter(categories)
    return min(counts, key=lambda category: (-counts[category], category.casefold()))


def special_group_key(record: dict[str, Any]) -> str:
    key = canonical_signature(record["name"])
    saline_key = canonical_signature("เสาน้ำเกลือสแตนเลส")
    if key != saline_key:
        return key
    walking = "ไม้เท้า/ไม้ค้ำ/พยุงเดิน/ROLLATOR/เก้าอี้นั่งถ่าย"
    if normalize_category(record["category"]) == walking:
        return key + ":walking"
    return key + ":bed"


def generated_price(category: str, group_key: str, branch_code: str) -> float:
    minimum, maximum = PRICE_RANGES.get(category, PRICE_RANGES[""])
    # Prices are generated in sensible 10-baht increments.
    return float(stable_int(f"price:{group_key}:{branch_code}", minimum // 10, maximum // 10) * 10)


def stock_quantities(price: float, group_key: str, branch_code: str) -> tuple[int, int]:
    if price <= 500:
        real_range, ghost_range = (10, 80), (3, 40)
    elif price <= 5000:
        real_range, ghost_range = (3, 30), (1, 15)
    else:
        real_range, ghost_range = (1, 8), (0, 4)
    real = stable_int(f"stock-real:{group_key}:{branch_code}", *real_range)
    ghost = stable_int(f"stock-ghost:{group_key}:{branch_code}", *ghost_range)
    if stable_int(f"out-of-stock:{group_key}:{branch_code}", 0, 99) < 10:
        if stable_int(f"empty-bucket:{group_key}:{branch_code}", 0, 1) == 0:
            real = 0
            ghost = max(1, ghost)
        else:
            ghost = 0
            real = max(1, real)
    if real == ghost:
        if ghost < ghost_range[1]:
            ghost += 1
        elif real < real_range[1]:
            real += 1
        else:
            ghost = max(0, ghost - 1)
    if real == ghost:
        raise ValueError(f"stock buckets unexpectedly equal for {group_key}/{branch_code}")
    return real, ghost


def image_merge_candidates(records: list[dict[str, Any]]) -> list[dict[str, Any]]:
    groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for record in records:
        if record["image_url"]:
            groups[record["image_url"]].append(record)
    candidates = []
    for image_url, items in groups.items():
        keys = {special_group_key(item) for item in items}
        if len(keys) < 2:
            continue
        names = sorted({item["name"] for item in items})
        if len(names) < 2:
            continue
        candidates.append(
            {
                "image_url": image_url,
                "names": names,
                "branch_codes": sorted({item["branch_code"] for item in items}),
                "reason": "ภาพเดียวกันแต่ชื่อไม่ผ่านกฎรวมแบบปลอดภัย จึงยังไม่รวมอัตโนมัติ",
            }
        )
    return sorted(candidates, key=lambda item: (item["names"], item["image_url"]))


def resolution_maps() -> tuple[dict[str, tuple[str, str]], dict[str, tuple[Any, ...]]]:
    merged: dict[str, tuple[str, str]] = {}
    for key, display_name, names in MERGE_GROUPS:
        for name in names:
            if name in merged:
                raise ValueError(f"duplicate merge resolution for {name}")
            merged[name] = (f"reviewed:{key}", display_name)
    variants: dict[str, tuple[Any, ...]] = {}
    for key, base_name, labels, names in VARIANT_GROUPS:
        for name in names:
            if name in variants:
                raise ValueError(f"duplicate variant resolution for {name}")
            selected = labels
            normalized = canonical_signature(name)
            if key == "oxygen-mask-bag" and normalized.endswith("ผู้ใหญ่"):
                selected = ("ผู้ใหญ่",)
            elif key == "oxygen-mask-bag" and normalized.endswith("เด็ก"):
                selected = ("เด็ก",)
            elif re.search(r"(?:size)?s$", normalized) or name.endswith("(M)") or " M (40" in name:
                if name.endswith("(M)") or " M (40" in name:
                    selected = tuple(label for label in labels if " M" in label)
                else:
                    selected = tuple(label for label in labels if label.endswith(" S"))
            elif name.endswith(" L"):
                selected = tuple(label for label in labels if label.endswith(" L"))
            variants[name] = (key, base_name, labels, selected)
    return merged, variants


def resolved_source_records(records: list[dict[str, Any]]) -> list[dict[str, Any]]:
    merged, variants = resolution_maps()
    resolved: list[dict[str, Any]] = []
    for source in records:
        name = source["name"]
        if name in variants:
            key, base_name, labels, selected = variants[name]
            for label in selected:
                if label not in labels:
                    raise ValueError(f"variant label {label!r} is not in {labels!r} for {name!r}")
                row = dict(source)
                suffix = canonical_signature(label)
                row["_group_key"] = f"reviewed:{key}:{suffix}"
                row["_canonical_name"] = f"{base_name} {label}"
                row["_force_zero_stock"] = True
                position = labels.index(label)
                base = stable_int(f"variant-price-base:{key}", 8, 30) * 10
                step = stable_int(f"variant-price-step:{key}", 3, 12) * 10
                row["_variant_price"] = float(base + position * step)
                if key == "stainless-measure":
                    measure_base = stable_int("measure-price-base", 12, 18) * 10
                    measure_step = stable_int("measure-price-step", 6, 10) * 10
                    row["_variant_price"] = float(measure_base + position * measure_step)
                resolved.append(row)
            continue
        row = dict(source)
        if name in merged:
            row["_group_key"], row["_canonical_name"] = merged[name]
        else:
            row["_group_key"] = special_group_key(row)
            row["_canonical_name"] = SEPARATE_NAME_OVERRIDES.get(name, "")
        resolved.append(row)
    return resolved


def mhtml_images(path: Path) -> dict[str, tuple[bytes, str]]:
    with path.open("rb") as handle:
        message = BytesParser(policy=policy.default).parse(handle)
    images: dict[str, tuple[bytes, str]] = {}
    for part in message.walk():
        if part.get_content_maintype() != "image":
            continue
        location = part.get("Content-Location", "")
        payload = part.get_payload(decode=True) or b""
        if location and payload:
            images.setdefault(location, (payload, part.get_content_type()))
    return images


def attach_product_images(manifest: dict[str, Any], repository_root: Path, asset_dir: Path | None) -> None:
    embedded: dict[str, tuple[bytes, str]] = {}
    for spec in SOURCE_SPECS:
        embedded.update(mhtml_images(repository_root / spec.product_file))

    existing_assets: dict[str, tuple[Path, str]] = {}
    if DEFAULT_OUTPUT.exists():
        try:
            previous = json.loads(DEFAULT_OUTPUT.read_text(encoding="utf-8"))
            previous_dir = DEFAULT_OUTPUT.parent / "ocha_images"
            for item in previous.get("product_images", []):
                path = previous_dir / item["asset_name"]
                if path.exists():
                    existing_assets[item["source_url"]] = (path, item["mime_type"])
        except (OSError, ValueError, KeyError):
            pass

    product_by_id = {item["id"]: item for item in manifest["products"]}
    candidates: dict[str, list[dict[str, Any]]] = defaultdict(list)
    asset_payloads: dict[str, bytes] = {}
    for source in manifest["source_records"]:
        source_url = source.get("image_url", "")
        if not source_url:
            continue
        payload_and_type = embedded.get(source_url)
        if payload_and_type is None and source_url in existing_assets:
            path, mime_type = existing_assets[source_url]
            payload_and_type = (path.read_bytes(), mime_type)
        if payload_and_type is None:
            try:
                request = Request(source_url, headers={"User-Agent": "pharmacy-erp-ocha-import/1.0"})
                with urlopen(request, timeout=15) as response:
                    payload = response.read(maxProductImageBytes + 1)
                    mime_type = response.headers.get_content_type()
                if len(payload) <= maxProductImageBytes and mime_type.startswith("image/"):
                    payload_and_type = (payload, mime_type)
            except Exception:
                payload_and_type = None
        if payload_and_type is None:
            continue
        payload, mime_type = payload_and_type
        extension = {"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}.get(mime_type)
        if extension is None:
            guessed = mimetypes.guess_extension(mime_type) or ""
            extension = ".jpg" if guessed in {".jpe", ".jpeg"} else guessed
        if extension not in {".jpg", ".png", ".webp"}:
            continue
        digest = hashlib.sha256(payload).hexdigest()
        asset_name = digest + extension
        storage_key = "ocha-" + asset_name
        asset_payloads[asset_name] = payload
        product_ids = source.get("product_ids") or [source["product_id"]]
        for product_id in product_ids:
            product_name = product_by_id[product_id]["name"]
            if product_name in SKIP_ALL_IMAGE_NAMES or (product_name, source["branch_code"]) in SKIP_IMAGE_BRANCHES:
                continue
            key = (storage_key, product_id)
            if any(item["storage_key"] == storage_key for item in candidates[product_id]):
                continue
            candidates[product_id].append(
                {
                    "id": stable_uuid("product-image", f"{product_id}:{digest}"),
                    "product_id": product_id,
                    "storage_key": storage_key,
                    "asset_name": asset_name,
                    "mime_type": mime_type,
                    "sha256": digest,
                    "source_url": source_url,
                    "source_branch_code": source["branch_code"],
                    "source_row_index": source["row_index"],
                    "source_name": source["name"],
                    "alt_text": product_name,
                    "_priority": (
                        0 if len(product_ids) == 1 else 1,
                        BRANCH_PRIORITY[source["branch_code"]],
                        source["row_index"],
                    ),
                }
            )

    rows: list[dict[str, Any]] = []
    for product_id, items in candidates.items():
        items.sort(key=lambda item: item["_priority"])
        for index, item in enumerate(items):
            item.pop("_priority")
            item["is_primary"] = index == 0
            item["sort_order"] = index
            rows.append(item)
    manifest["product_images"] = sorted(rows, key=lambda item: (item["product_id"], item["sort_order"]))
    manifest["summary"]["product_images"] = len(rows)
    manifest["summary"]["products_with_images"] = len(candidates)
    manifest["summary"]["products_with_placeholder"] = len(manifest["products"]) - len(candidates)

    if asset_dir is not None:
        asset_dir.mkdir(parents=True, exist_ok=True)
        expected = set(asset_payloads)
        for old in asset_dir.iterdir():
            if old.is_file() and old.name not in expected:
                old.unlink()
        for name, payload in asset_payloads.items():
            target = asset_dir / name
            if not target.exists() or target.read_bytes() != payload:
                target.write_bytes(payload)


def median_int(values: Iterable[int]) -> int:
    values = list(values)
    return int(round(statistics.median(values))) if values else 0


def build_manifest(repository_root: Path, asset_dir: Path | None = None) -> dict[str, Any]:
    source_records: list[dict[str, Any]] = []
    source_files: list[dict[str, Any]] = []
    category_names: set[str] = set()

    for spec in SOURCE_SPECS:
        product_path = repository_root / spec.product_file
        category_path = repository_root / spec.category_file
        products = extract_products(product_path, spec.code)
        categories = extract_categories(category_path)
        if len(products) != spec.expected_products:
            raise ValueError(f"{spec.code}: expected {spec.expected_products} products, got {len(products)}")
        if len(categories) != spec.expected_categories:
            raise ValueError(f"{spec.code}: expected {spec.expected_categories} categories, got {len(categories)}")
        source_records.extend(products)
        category_names.update(normalize_category(item["name"]) for item in categories)
        source_files.extend(
            [
                {
                    "branch_code": spec.code,
                    "kind": "products",
                    "path": spec.product_file,
                    "sha256": sha256(product_path),
                    "row_count": len(products),
                },
                {
                    "branch_code": spec.code,
                    "kind": "categories",
                    "path": spec.category_file,
                    "sha256": sha256(category_path),
                    "row_count": len(categories),
                },
            ]
        )

    category_names.discard("")
    category_rows = []
    category_ids: dict[str, str] = {}
    for index, category_name in enumerate(sorted(category_names, key=str.casefold)):
        category_id = stable_uuid("category", category_name)
        category_ids[category_name] = category_id
        category_rows.append(
            {
                "id": category_id,
                "name": category_name,
                "color": CATEGORY_COLORS[index % len(CATEGORY_COLORS)],
                "active": True,
            }
        )
    if len(category_rows) != 12:
        raise ValueError(f"expected 12 canonical categories, got {len(category_rows)}")

    branch_ids = {spec.code: stable_uuid("branch", spec.code) for spec in SOURCE_SPECS}
    branch_ids["WH"] = stable_uuid("branch", "WH")
    branches = [
        {
            "id": branch_ids["WH"],
            "code": "WH",
            "name": "โกดัง",
            "address": "",
            "branch_type": "main_warehouse",
            "parent_branch_id": None,
            "active": True,
            "sales_enabled": False,
        }
    ]
    for spec in SOURCE_SPECS:
        branches.append(
            {
                "id": branch_ids[spec.code],
                "code": spec.code,
                "name": spec.name,
                "address": "",
                "branch_type": spec.branch_type,
                "parent_branch_id": branch_ids["WH"],
                "active": True,
                "sales_enabled": True,
            }
        )
    branches.sort(key=lambda item: (item["branch_type"] != "main_warehouse", item["code"]))

    grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
    resolved_records = resolved_source_records(source_records)
    for record in resolved_records:
        grouped[record["_group_key"]].append(record)

    products: list[dict[str, Any]] = []
    branch_prices: list[dict[str, Any]] = []
    branch_settings: list[dict[str, Any]] = []
    inventory_rows: list[dict[str, Any]] = []
    inventory_lots: list[dict[str, Any]] = []
    source_to_products: dict[tuple[str, int], list[tuple[str, str]]] = defaultdict(list)
    used_barcodes: set[str] = set()

    for group_key in sorted(grouped):
        records = grouped[group_key]
        canonical_name = next((record["_canonical_name"] for record in records if record.get("_canonical_name")), choose_name(records))
        category_name = choose_category(group_key, canonical_name, records)
        if group_key.endswith(":walking"):
            canonical_name = "เสาน้ำเกลือสแตนเลส (หมวดอุปกรณ์ช่วยเดิน)"
        elif group_key.endswith(":bed"):
            canonical_name = "เสาน้ำเกลือสแตนเลส (หมวดเตียงผู้ป่วย)"
        product_id = stable_uuid("product", group_key)
        sku = "OCH-" + hashlib.sha256(group_key.encode("utf-8")).hexdigest()[:10].upper()

        records_by_branch: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for record in records:
            records_by_branch[record["branch_code"]].append(record)
            source_to_products[(record["branch_code"], record["row_index"])].append((product_id, group_key))

        effective_prices: dict[str, float] = {}
        for branch_code, branch_records in records_by_branch.items():
            numeric_prices = [parse_price(record["price_label"]) for record in branch_records]
            numeric_prices = [price for price in numeric_prices if price is not None]
            if len(set(numeric_prices)) > 1:
                raise ValueError(f"unresolved price conflict for {group_key}/{branch_code}: {numeric_prices}")
            variant_prices = [record.get("_variant_price") for record in branch_records if record.get("_variant_price") is not None]
            effective_prices[branch_code] = numeric_prices[0] if numeric_prices else (variant_prices[0] if variant_prices else generated_price(category_name, group_key, branch_code))

        base_price = round_money(statistics.median(effective_prices.values()))
        cost_ratio = stable_int(f"cost-ratio:{group_key}", 55, 75) / 100
        retail_ratio = stable_int(f"retail-ratio:{group_key}", 105, 120) / 100
        discount_ratio = stable_int(f"discount-ratio:{group_key}", 5, 12) / 100
        cost_price = round_money(base_price * cost_ratio)
        retail_price = round_money(base_price * retail_ratio)
        max_discount = round_money(base_price * discount_ratio)
        warning_days = (30, 60, 90)[stable_int(f"warning:{group_key}", 0, 2)]

        per_branch_thresholds: list[tuple[int, int]] = []
        for branch_code in sorted(records_by_branch):
            branch_id = branch_ids[branch_code]
            selling_price = round_money(effective_prices[branch_code])
            branch_prices.append(
                {
                    "id": stable_uuid("branch-price", f"{branch_code}:{group_key}"),
                    "branch_id": branch_id,
                    "product_id": product_id,
                    "selling_price": selling_price,
                }
            )
            if any(record.get("_force_zero_stock") for record in records):
                qty_real, qty_ghost = 0, 0
            else:
                qty_real, qty_ghost = stock_quantities(selling_price, group_key, branch_code)
            inventory_id = stable_uuid("inventory", f"{branch_code}:{group_key}")
            inventory_rows.append(
                {
                    "id": inventory_id,
                    "branch_id": branch_id,
                    "product_id": product_id,
                    "qty_real": qty_real,
                    "qty_ghost": qty_ghost,
                }
            )
            real_threshold = max(1, round(qty_real * 0.2))
            ghost_threshold = max(1, round(qty_ghost * 0.2))
            per_branch_thresholds.append((real_threshold, ghost_threshold))
            branch_settings.append(
                {
                    "id": stable_uuid("branch-product-setting", f"{branch_code}:{group_key}"),
                    "branch_id": branch_id,
                    "product_id": product_id,
                    "max_discount_amount": round_money(selling_price * discount_ratio),
                    "low_stock_real_threshold": real_threshold,
                    "low_stock_ghost_threshold": ghost_threshold,
                }
            )
            expires_on = GENERATED_ON + timedelta(days=stable_int(f"expiry:{group_key}:{branch_code}", 90, 730))
            for stock_bucket, quantity, suffix in (("real", qty_real, "R"), ("ghost", qty_ghost, "G")):
                if quantity <= 0:
                    continue
                inventory_lots.append(
                    {
                        "id": stable_uuid("inventory-lot", f"{branch_code}:{group_key}:{stock_bucket}"),
                        "branch_id": branch_id,
                        "product_id": product_id,
                        "stock_bucket": stock_bucket,
                        "lot_number": f"OPEN-{branch_code}-{sku[-10:]}-{suffix}",
                        "expires_on": expires_on.isoformat(),
                        "received_quantity": quantity,
                        "remaining_quantity": quantity,
                        "unit_cost": cost_price,
                        "source_type": "ocha_seed_opening",
                        "source_id": None,
                        "source_item_id": None,
                        "origin_lot_id": None,
                        "received_at": datetime.combine(GENERATED_ON, datetime.min.time(), timezone.utc).isoformat().replace("+00:00", "Z"),
                    }
                )

        price_labels = sorted({record["price_label"] for record in records if "ราคา" in record["price_label"]})
        description = "นำเข้าจาก Ocha POS"
        if price_labels:
            description += "; ป้ายราคาเดิม: " + ", ".join(price_labels)
        products.append(
            {
                "id": product_id,
                "sku": sku,
                "barcode": ean13_for(group_key, used_barcodes),
                "category_id": category_ids.get(category_name),
                "name": canonical_name,
                "description": description,
                "cost_price": cost_price,
                "base_selling_price": base_price,
                "retail_price": retail_price,
                "max_discount_amount": max_discount,
                "low_stock_real_threshold": median_int(pair[0] for pair in per_branch_thresholds),
                "low_stock_ghost_threshold": median_int(pair[1] for pair in per_branch_thresholds),
                "tracks_expiry": True,
                "expiry_warning_days": warning_days,
                "unit_name": "ชิ้น",
                "tax_exempt": False,
                "active": True,
            }
        )

    # The warehouse owns the complete catalog. Its opening stock is an explicit
    # copy of current MES balances; all other global products start at zero.
    mes_inventory = {
        item["product_id"]: item
        for item in inventory_rows
        if item["branch_id"] == branch_ids["MES"]
    }
    mes_lots = [lot for lot in inventory_lots if lot["branch_id"] == branch_ids["MES"]]
    for product in products:
        source_inventory = mes_inventory.get(product["id"])
        inventory_rows.append(
            {
                "id": stable_uuid("inventory", f"WH:{product['id']}"),
                "branch_id": branch_ids["WH"],
                "product_id": product["id"],
                "qty_real": source_inventory["qty_real"] if source_inventory else 0,
                "qty_ghost": source_inventory["qty_ghost"] if source_inventory else 0,
            }
        )
    for source_lot in mes_lots:
        if source_lot["remaining_quantity"] <= 0:
            continue
        inventory_lots.append(
            {
                **source_lot,
                "id": stable_uuid("warehouse-lot-copy", source_lot["id"]),
                "branch_id": branch_ids["WH"],
                "received_quantity": source_lot["remaining_quantity"],
                "source_type": "warehouse_bootstrap_copy",
                "source_id": None,
                "source_item_id": None,
                "origin_lot_id": source_lot["id"],
            }
        )

    enriched_sources = []
    for record in source_records:
        resolved_products = source_to_products[(record["branch_code"], record["row_index"])]
        product_ids = [item[0] for item in resolved_products]
        canonical_keys = [item[1] for item in resolved_products]
        enriched_sources.append(
            {
                **record,
                "canonical_key": canonical_keys[0],
                "canonical_keys": canonical_keys,
                "product_id": product_ids[0],
                "product_ids": product_ids,
                "match_method": "reviewed_resolution" if canonical_keys[0].startswith("reviewed:") else "safe_normalized_name",
            }
        )

    alias_by_key: dict[tuple[str, str], dict[str, Any]] = {}
    for source in enriched_sources:
        for product_id in source["product_ids"]:
            alias_key = (product_id, canonical_signature(source["name"]))
            alias_by_key.setdefault(
                alias_key,
                {
                    "id": stable_uuid("product-source-alias", ":".join(alias_key)),
                    "product_id": product_id,
                    "alias_name": source["name"],
                    "normalized_alias": alias_key[1],
                    "source_branch_code": source["branch_code"],
                    "source_row_index": source["row_index"],
                },
            )
    product_name_aliases = sorted(alias_by_key.values(), key=lambda item: (item["product_id"], item["normalized_alias"]))

    manifest = {
        "schema_version": 1,
        "generated_at": datetime.combine(GENERATED_ON, datetime.min.time(), timezone.utc).isoformat().replace("+00:00", "Z"),
        "random_seed": RANDOM_SEED,
        "source_files": source_files,
        "summary": {
            "source_records": len(enriched_sources),
            "branches": len(branches),
            "product_categories": len(category_rows),
            "products": len(products),
            "branch_product_memberships": len(inventory_rows),
            "branch_product_prices": len(branch_prices),
            "inventory_lots": len(inventory_lots),
            "multi_price_source_records": sum("ราคา" in item["price_label"] for item in enriched_sources),
            "uncategorized_source_records": sum(item["category"] == "-" for item in enriched_sources),
        },
        "branches": branches,
        "product_categories": sorted(category_rows, key=lambda item: item["name"].casefold()),
        "products": sorted(products, key=lambda item: item["sku"]),
        "branch_product_prices": sorted(branch_prices, key=lambda item: (item["branch_id"], item["product_id"])),
        "branch_product_settings": sorted(branch_settings, key=lambda item: (item["branch_id"], item["product_id"])),
        "inventory": sorted(inventory_rows, key=lambda item: (item["branch_id"], item["product_id"])),
        "inventory_lots": sorted(inventory_lots, key=lambda item: (item["branch_id"], item["product_id"], item["stock_bucket"])),
        "product_name_aliases": product_name_aliases,
        "source_records": sorted(enriched_sources, key=lambda item: (BRANCH_PRIORITY[item["branch_code"]], item["row_index"])),
        "merge_candidates": [],
        "merge_resolutions": [
            {"key": key, "canonical_name": name, "source_names": list(names), "resolution": "merge"}
            for key, name, names in MERGE_GROUPS
        ] + [
            {"key": key, "canonical_name": base_name, "variants": list(labels), "source_names": list(names), "resolution": "variants"}
            for key, base_name, labels, names in VARIANT_GROUPS
        ],
    }
    manifest["summary"]["product_name_aliases"] = len(product_name_aliases)
    attach_product_images(manifest, repository_root, asset_dir)
    validate_manifest(manifest)
    return manifest


def validate_manifest(manifest: dict[str, Any]) -> None:
    def unique(rows: list[dict[str, Any]], field: str) -> None:
        values = [row[field] for row in rows]
        if len(values) != len(set(values)):
            raise ValueError(f"duplicate {field}")

    unique(manifest["branches"], "id")
    unique(manifest["products"], "id")
    unique(manifest["products"], "sku")
    unique(manifest["products"], "barcode")
    unique(manifest["product_categories"], "id")
    unique(manifest["product_images"], "id")
    unique(manifest["product_name_aliases"], "id")
    if any(not product["tracks_expiry"] for product in manifest["products"]):
        raise ValueError("every imported product must track expiry")
    if any(item["qty_real"] == item["qty_ghost"] and item["qty_real"] != 0 for item in manifest["inventory"]):
        raise ValueError("positive real and ghost seed quantities must differ")
    product_ids = {product["id"] for product in manifest["products"]}
    if any(alias["product_id"] not in product_ids or not alias["normalized_alias"] for alias in manifest["product_name_aliases"]):
        raise ValueError("invalid product source alias")
    primary_counts: Counter[str] = Counter()
    for image in manifest["product_images"]:
        if image["product_id"] not in product_ids or len(image["sha256"]) != 64:
            raise ValueError("invalid product image reference")
        primary_counts[image["product_id"]] += int(image["is_primary"])
    if any(count != 1 for count in primary_counts.values()):
        raise ValueError("each imaged product must have exactly one primary image")
    inventory = {(row["branch_id"], row["product_id"]): row for row in manifest["inventory"]}
    lot_totals: dict[tuple[str, str, str], int] = defaultdict(int)
    for lot in manifest["inventory_lots"]:
        if not lot["expires_on"]:
            raise ValueError("every positive opening lot must expire")
        lot_totals[(lot["branch_id"], lot["product_id"], lot["stock_bucket"])] += lot["remaining_quantity"]
    for key, row in inventory.items():
        if lot_totals.get((*key, "real"), 0) != row["qty_real"]:
            raise ValueError(f"real lot mismatch for {key}")
        if lot_totals.get((*key, "ghost"), 0) != row["qty_ghost"]:
            raise ValueError(f"ghost lot mismatch for {key}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repository-root", type=Path, default=REPOSITORY_ROOT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--asset-dir", type=Path, default=DEFAULT_OUTPUT.parent / "ocha_images")
    parser.add_argument("--check", action="store_true", help="fail if output differs; do not write")
    args = parser.parse_args()
    manifest = build_manifest(args.repository_root.resolve(), None if args.check else args.asset_dir.resolve())
    encoded = json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=False) + "\n"
    output = args.output.resolve()
    if args.check:
        if not output.exists() or output.read_text(encoding="utf-8") != encoded:
            raise SystemExit(f"generated seed differs from {output}")
    else:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(encoded, encoding="utf-8")
    print(json.dumps(manifest["summary"], ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
