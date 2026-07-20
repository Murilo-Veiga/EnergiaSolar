// Espelha fmtNum/fmtBRL em templates/index.html (Python) EXATAMENTE —
// Number.toFixed(), sem toLocaleString: o original não usa separador de
// milhar nem vírgula decimal, só ponto (ex.: "24.9", não "24,9").
export function fmtNum(v: number | null | undefined, digits = 1): string {
  return v === null || v === undefined ? "--" : Number(v).toFixed(digits);
}

export function fmtBRL(v: number | null | undefined): string {
  return v === null || v === undefined ? "--" : `R$ ${Number(v).toFixed(2).replace(".", ",")}`;
}
