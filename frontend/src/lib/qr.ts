const QR_SIZE = 21;
const DATA_CODEWORDS = 19;
const ECC_CODEWORDS = 7;
const PAD_BYTES = [0xec, 0x11];
const GENERATOR = [87, 229, 146, 149, 238, 102, 21];
const QUIET_ZONE = 4;

type Matrix = boolean[][];

const EXP_TABLE = new Array<number>(512).fill(0);
const LOG_TABLE = new Array<number>(256).fill(0);

let gfReady = false;

function initTables() {
  if (gfReady) {
    return;
  }
  let value = 1;
  for (let index = 0; index < 255; index += 1) {
    EXP_TABLE[index] = value;
    LOG_TABLE[value] = index;
    value <<= 1;
    if (value & 0x100) {
      value ^= 0x11d;
    }
  }
  for (let index = 255; index < EXP_TABLE.length; index += 1) {
    EXP_TABLE[index] = EXP_TABLE[index - 255];
  }
  gfReady = true;
}

function gfMultiply(left: number, right: number) {
  if (left === 0 || right === 0) {
    return 0;
  }
  initTables();
  return EXP_TABLE[LOG_TABLE[left] + LOG_TABLE[right]];
}

function appendBits(target: number[], value: number, width: number) {
  for (let shift = width - 1; shift >= 0; shift -= 1) {
    target.push((value >>> shift) & 1);
  }
}

function pushBit(target: number[], value: number) {
  target.push(value ? 1 : 0);
}

function bytesToBits(bytes: number[]) {
  const bits: number[] = [];
  bytes.forEach((value) => appendBits(bits, value, 8));
  return bits;
}

function buildDataCodewords(text: string) {
  const bytes = Array.from(new TextEncoder().encode(text));
  // System-generated and seeded transfer codes in this ERP fit QR version 1-L, which keeps the renderer dependency-free.
  if (bytes.length === 0 || bytes.length > 17) {
    return null;
  }

  const bits: number[] = [];
  appendBits(bits, 0b0100, 4);
  appendBits(bits, bytes.length, 8);
  bytes.forEach((value) => appendBits(bits, value, 8));

  const capacityBits = DATA_CODEWORDS * 8;
  const terminator = Math.min(4, capacityBits - bits.length);
  for (let index = 0; index < terminator; index += 1) {
    pushBit(bits, 0);
  }
  while (bits.length % 8 !== 0) {
    pushBit(bits, 0);
  }

  const codewords: number[] = [];
  for (let index = 0; index < bits.length; index += 8) {
    let value = 0;
    for (let offset = 0; offset < 8; offset += 1) {
      value = (value << 1) | bits[index + offset];
    }
    codewords.push(value);
  }

  const initialLength = codewords.length;
  for (let index = initialLength; index < DATA_CODEWORDS; index += 1) {
    codewords.push(PAD_BYTES[(index - initialLength) % PAD_BYTES.length]);
  }
  return codewords;
}

function buildErrorCorrection(dataCodewords: number[]) {
  const remainder = new Array<number>(ECC_CODEWORDS).fill(0);
  dataCodewords.forEach((value) => {
    const factor = value ^ remainder[0];
    for (let index = 0; index < ECC_CODEWORDS - 1; index += 1) {
      remainder[index] = remainder[index + 1] ^ gfMultiply(GENERATOR[index], factor);
    }
    remainder[ECC_CODEWORDS - 1] = gfMultiply(GENERATOR[ECC_CODEWORDS - 1], factor);
  });
  return remainder;
}

function createMatrix(defaultValue = false): Matrix {
  return Array.from({ length: QR_SIZE }, () => Array.from({ length: QR_SIZE }, () => defaultValue));
}

function drawFinder(modules: Matrix, reserved: Matrix, row: number, col: number) {
  for (let rowOffset = -1; rowOffset <= 7; rowOffset += 1) {
    for (let colOffset = -1; colOffset <= 7; colOffset += 1) {
      const targetRow = row + rowOffset;
      const targetCol = col + colOffset;
      if (targetRow < 0 || targetRow >= QR_SIZE || targetCol < 0 || targetCol >= QR_SIZE) {
        continue;
      }

      const isCore =
        rowOffset >= 0 &&
        rowOffset <= 6 &&
        colOffset >= 0 &&
        colOffset <= 6 &&
        (rowOffset === 0 ||
          rowOffset === 6 ||
          colOffset === 0 ||
          colOffset === 6 ||
          (rowOffset >= 2 && rowOffset <= 4 && colOffset >= 2 && colOffset <= 4));

      modules[targetRow][targetCol] = isCore;
      reserved[targetRow][targetCol] = true;
    }
  }
}

function reserveFormatAreas(reserved: Matrix) {
  for (let index = 0; index <= 5; index += 1) {
    reserved[8][index] = true;
    reserved[index][8] = true;
  }
  reserved[8][7] = true;
  reserved[8][8] = true;
  reserved[7][8] = true;
  for (let index = 9; index <= 14; index += 1) {
    reserved[14 - index][8] = true;
  }
  for (let index = 0; index <= 7; index += 1) {
    reserved[QR_SIZE - 1 - index][8] = true;
  }
  for (let index = 8; index <= 14; index += 1) {
    reserved[8][QR_SIZE - 15 + index] = true;
  }
}

function drawTiming(modules: Matrix, reserved: Matrix) {
  for (let index = 8; index < QR_SIZE - 8; index += 1) {
    const value = index % 2 === 0;
    if (!reserved[6][index]) {
      modules[6][index] = value;
      reserved[6][index] = true;
    }
    if (!reserved[index][6]) {
      modules[index][6] = value;
      reserved[index][6] = true;
    }
  }
}

function buildFormatBits(mask: number) {
  const data = (1 << 3) | mask;
  let remainder = data;
  for (let index = 0; index < 10; index += 1) {
    remainder = (remainder << 1) ^ (((remainder >>> 9) & 1) * 0x537);
  }
  return ((data << 10) | remainder) ^ 0x5412;
}

function getBit(value: number, bit: number) {
  return ((value >>> bit) & 1) === 1;
}

function drawFormatBits(modules: Matrix, mask: number) {
  const bits = buildFormatBits(mask);
  for (let index = 0; index <= 5; index += 1) {
    modules[8][index] = getBit(bits, index);
  }
  modules[8][7] = getBit(bits, 6);
  modules[8][8] = getBit(bits, 7);
  modules[7][8] = getBit(bits, 8);
  for (let index = 9; index <= 14; index += 1) {
    modules[14 - index][8] = getBit(bits, index);
  }
  for (let index = 0; index <= 7; index += 1) {
    modules[QR_SIZE - 1 - index][8] = getBit(bits, index);
  }
  for (let index = 8; index <= 14; index += 1) {
    modules[8][QR_SIZE - 15 + index] = getBit(bits, index);
  }
}

function placeData(modules: Matrix, reserved: Matrix, bits: number[]) {
  let cursor = 0;
  for (let right = QR_SIZE - 1; right >= 1; right -= 2) {
    if (right === 6) {
      right = 5;
    }
    for (let vertical = 0; vertical < QR_SIZE; vertical += 1) {
      for (let columnOffset = 0; columnOffset < 2; columnOffset += 1) {
        const col = right - columnOffset;
        const upward = ((right + 1) & 2) === 0;
        const row = upward ? QR_SIZE - 1 - vertical : vertical;
        if (reserved[row][col]) {
          continue;
        }
        const rawBit = cursor < bits.length ? bits[cursor] === 1 : false;
        const masked = (row + col) % 2 === 0 ? !rawBit : rawBit;
        modules[row][col] = masked;
        cursor += 1;
      }
    }
  }
}

export function buildQRCodeMatrix(text: string) {
  const dataCodewords = buildDataCodewords(text);
  if (!dataCodewords) {
    return null;
  }

  const eccCodewords = buildErrorCorrection(dataCodewords);
  const bits = bytesToBits(dataCodewords.concat(eccCodewords));
  const modules = createMatrix(false);
  const reserved = createMatrix(false);

  drawFinder(modules, reserved, 0, 0);
  drawFinder(modules, reserved, 0, QR_SIZE - 7);
  drawFinder(modules, reserved, QR_SIZE - 7, 0);
  drawTiming(modules, reserved);
  reserveFormatAreas(reserved);
  modules[QR_SIZE - 8][8] = true;
  reserved[QR_SIZE - 8][8] = true;
  placeData(modules, reserved, bits);
  drawFormatBits(modules, 0);

  return modules;
}

export function buildQRCodeSVG(text: string) {
  const modules = buildQRCodeMatrix(text);
  if (!modules) {
    return null;
  }

  const dimension = QR_SIZE + QUIET_ZONE * 2;
  const rects: string[] = [];
  modules.forEach((row, rowIndex) => {
    row.forEach((value, colIndex) => {
      if (!value) {
        return;
      }
      rects.push(`<rect x="${colIndex + QUIET_ZONE}" y="${rowIndex + QUIET_ZONE}" width="1" height="1" />`);
    });
  });

  return [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${dimension} ${dimension}" width="100%" height="100%" shape-rendering="crispEdges">`,
    `<rect width="${dimension}" height="${dimension}" fill="white" />`,
    `<g fill="black">`,
    rects.join(""),
    `</g>`,
    `</svg>`
  ].join("");
}
