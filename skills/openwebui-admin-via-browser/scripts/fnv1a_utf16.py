"""Хэш FNV-1a (32 бита) по кодовым единицам UTF-16, как у функции fnv1a в SKILL.md.

Переводы строк приводятся к \n, BOM в начале файла отбрасывается, поэтому
локальный файл с LF и код на сервере с CRLF дают одинаковый хэш.

Запуск: python fnv1a_utf16.py <файл>
"""
import sys


def fnv1a_utf16(text: str) -> str:
    text = text.replace("\r\n", "\n")
    data = text.encode("utf-16-le")
    h = 0x811C9DC5
    for i in range(0, len(data), 2):
        h ^= data[i] | (data[i + 1] << 8)
        h = (h * 0x01000193) & 0xFFFFFFFF
    return format(h, "08x")


def main() -> int:
    if len(sys.argv) != 2:
        print("Запуск: python fnv1a_utf16.py <файл>", file=sys.stderr)
        return 2
    with open(sys.argv[1], encoding="utf-8-sig", newline="") as f:
        print(fnv1a_utf16(f.read()))
    return 0


if __name__ == "__main__":
    sys.exit(main())
