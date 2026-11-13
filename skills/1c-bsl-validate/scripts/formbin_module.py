"""Действующий module из 32-битного контейнера Form.bin, без записи файлов."""
import re
import struct
import zlib

VERSION = '1.0.0'
END = 0x7fffffff
_HEADER = re.compile(rb'\r\n([0-9a-fA-F]{8}) ([0-9a-fA-F]{8}) ([0-9a-fA-F]{8}) \r\n')


class FormBinError(ValueError):
    """Поврежденный или неподдержанный контейнер обычной формы."""


def _chain(data, position):
    visited = set()
    chunks = []
    remaining = None
    while True:
        if position in visited:
            raise FormBinError('Цикл в цепочке блоков')
        visited.add(position)
        if position < 16 or position + 31 > len(data):
            raise FormBinError('Адрес блока вне границ контейнера')
        match = _HEADER.match(data, position)
        if not match:
            raise FormBinError('Неподдержанный или поврежденный заголовок блока, ожидается 32-битный формат')
        full, size, following = (int(value, 16) for value in match.groups())
        body = match.end()
        if body + size > len(data):
            raise FormBinError('Данные блока выходят за границы контейнера')
        if remaining is None:
            remaining = full
        if size == 0 and remaining:
            raise FormBinError('Пустой блок незавершенной цепочки')
        count = min(size, remaining)
        chunks.append(data[body:body + count])
        remaining -= count
        if following == END:
            if remaining:
                raise FormBinError('Цепочка оборвана до объявленного размера')
            return b''.join(chunks)
        if remaining == 0:
            raise FormBinError('Лишнее продолжение завершенной цепочки')
        position = following


def _module_text(body):
    # Несжатый UTF-8 и raw deflate встречаются в одном формате контейнера.
    try:
        decoder = zlib.decompressobj(-15)
        expanded = decoder.decompress(body) + decoder.flush()
    except zlib.error:
        expanded = None
    if expanded is not None and decoder.eof:
        if decoder.unused_data or decoder.unconsumed_tail:
            raise FormBinError('Лишние данные после сжатого module')
        body = expanded
    try:
        text = body.decode('utf-8-sig')
    except UnicodeDecodeError as error:
        raise FormBinError('module не является поддержанным UTF-8 или raw deflate') from error
    if any(ord(char) < 32 and char not in '\r\n\t' for char in text):
        raise FormBinError('module содержит двоичные управляющие символы')
    return text


def read_form_module(data: bytes) -> str:
    """Возвращает только действующий текст module из оглавления в памяти.

    Отсутствующий module, повреждение и неподдержанный формат дают FormBinError.
    Имена элементов контейнера никогда не используются как пути файлов.
    """
    if len(data) < 47:
        raise FormBinError('Слишком короткий контейнер Form.bin')
    first_free, _, _, _ = struct.unpack_from('<IIII', data)
    if first_free != END and not 16 <= first_free < len(data):
        raise FormBinError('Указатель свободного блока вне границ контейнера')
    toc = _chain(data, 16)
    if len(toc) % 12:
        raise FormBinError('Повреждено 32-битное оглавление контейнера')
    module = None
    for offset in range(0, len(toc), 12):
        header_address, body_address, _ = struct.unpack_from('<III', toc, offset)
        if header_address == END:
            continue
        metadata = _chain(data, header_address)
        if len(metadata) < 20 or len(metadata[20:]) % 2:
            raise FormBinError('Повреждены метаданные элемента контейнера')
        try:
            name = metadata[20:].decode('utf-16-le').split('\0', 1)[0]
        except UnicodeDecodeError as error:
            raise FormBinError('Имя элемента не является UTF-16LE') from error
        if name != 'module':
            continue
        if module is not None:
            raise FormBinError('В оглавлении несколько элементов module')
        module = _module_text(_chain(data, body_address))
    if module is None:
        raise FormBinError('В оглавлении отсутствует module')
    return module
