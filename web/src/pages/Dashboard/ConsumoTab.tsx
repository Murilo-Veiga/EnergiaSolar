import { useEffect, useState, type ChangeEvent } from "react";
import { useActivePlant } from "../../context/PlantContext";
import {
  api,
  ApiError,
  type ConsumptionHistoryRow,
  type ConsumptionSummary,
  type ConsumptionUploadResult,
} from "../../lib/api";
import { fmtNum, fmtBRL } from "../../lib/fmt";
import { IconBadge } from "../../components/icons";
import { Collapsible } from "../../components/Collapsible";

export function ConsumoTab() {
  const plant = useActivePlant();
  const [summary, setSummary] = useState<ConsumptionSummary | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [uploadResult, setUploadResult] = useState<ConsumptionUploadResult | null>(null);
  const [selectedUC, setSelectedUC] = useState<string | null>(null);
  const [history, setHistory] = useState<ConsumptionHistoryRow[]>([]);

  async function loadSummary() {
    const s = await api.get<ConsumptionSummary>(`/api/plants/${plant.id}/consumption/summary`);
    setSummary(s);
    if (!selectedUC && s.unidades.length > 0) {
      setSelectedUC(s.unidades[0].uc_number);
    }
  }

  useEffect(() => {
    void loadSummary();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [plant.id]);

  useEffect(() => {
    if (!selectedUC) {
      setHistory([]);
      return;
    }
    void api
      .get<{ rows: ConsumptionHistoryRow[] }>(`/api/plants/${plant.id}/consumption/history?uc=${encodeURIComponent(selectedUC)}`)
      .then((r) => setHistory(r.rows));
  }, [plant.id, selectedUC]);

  async function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;

    setUploading(true);
    setUploadError(null);
    setUploadResult(null);
    try {
      const formData = new FormData();
      formData.append("file", file);
      const result = await api.upload<ConsumptionUploadResult>(`/api/plants/${plant.id}/consumption/upload`, formData);
      setUploadResult(result);
      setSelectedUC(result.uc);
      await loadSummary();
    } catch (err) {
      setUploadError(err instanceof ApiError ? err.message : "falha ao importar a fatura");
    } finally {
      setUploading(false);
    }
  }

  return (
    <div>
      <div className="card" style={{ padding: "16px 20px", marginBottom: 14, display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
        <p style={{ margin: 0, fontSize: 12.5, color: "var(--ink-muted)", maxWidth: "60ch", lineHeight: 1.6 }}>
          Importe o PDF da sua fatura da Celesc pra comparar o que você pagou com o que sua instalação gerou — a fatura já traz até 13
          meses de histórico de consumo, importados de uma vez só.
        </p>
        <label className="btn-upload" style={{ cursor: uploading ? "wait" : "pointer" }}>
          {uploading ? "Importando…" : "📄 Importar fatura (PDF)"}
          <input type="file" accept="application/pdf" hidden disabled={uploading} onChange={handleFileChange} />
        </label>
      </div>

      {uploadError && (
        <div className="card" style={{ padding: "12px 20px", marginBottom: 14, color: "var(--bad, #e05a5a)", fontSize: 13 }}>
          {uploadError}
        </div>
      )}
      {uploadResult && (
        <div className="card" style={{ padding: "12px 20px", marginBottom: 14, fontSize: 13, color: "var(--ink-2)" }}>
          Fatura de {uploadResult.referencia} importada ({fmtNum(uploadResult.consumo_kwh, 0)} kWh
          {uploadResult.total_pagar_brl !== null ? `, ${fmtBRL(uploadResult.total_pagar_brl)}` : ""}
          {uploadResult.meses_historico_importados > 0
            ? ` — mais ${uploadResult.meses_historico_importados} mês(es) de histórico`
            : ""}
          ).
        </div>
      )}

      {summary && summary.unidades.length === 0 && (
        <div className="card placeholder-tab">
          <IconBadge name="wallet" color="aqua" size="card" />
          <h3 style={{ margin: 0, color: "var(--ink-2)" }}>Nenhuma fatura importada ainda</h3>
          <p style={{ maxWidth: 380, fontSize: 13 }}>Importe o PDF da sua fatura da Celesc acima pra começar.</p>
        </div>
      )}

      {summary && summary.unidades.length > 0 && (
        <Collapsible
          icon="wallet"
          iconColor="aqua"
          title="Economia estimada"
          tooltip="Geração acumulada da usina × tarifa efetiva (valor pago ÷ kWh) da fatura mais recente — não é o valor oficial da Celesc, é aproximação."
          defaultOpen
        >
          <div className="chart-total-value" style={{ textAlign: "left" }}>
            {fmtBRL(summary.economia_estimada_brl)}
          </div>
          <div className="chart-caption" style={{ marginTop: 6 }}>
            Estimativa desde que a usina começou a gerar, baseada na tarifa da fatura mais recente.
          </div>
        </Collapsible>
      )}

      {summary && summary.unidades.length > 0 && (
        <div className="day-grid" style={{ gridTemplateColumns: `repeat(${Math.min(summary.unidades.length, 3)}, 1fr)`, marginBottom: 14 }}>
          {summary.unidades.map((u) => (
            <div
              key={u.uc_number}
              className="card"
              style={{
                padding: "14px 16px",
                cursor: "pointer",
                border: selectedUC === u.uc_number ? "1px solid var(--accent)" : undefined,
              }}
              onClick={() => setSelectedUC(u.uc_number)}
            >
              <div className="l">{u.label}</div>
              <div className="v">
                {u.latest ? `${fmtNum(u.latest.consumed_kwh, 0)} kWh · ${u.latest.referencia}` : "sem fatura"}
              </div>
              {u.latest?.total_value_brl != null && (
                <div style={{ fontSize: 12, color: "var(--ink-muted)", marginTop: 4 }}>{fmtBRL(u.latest.total_value_brl)}</div>
              )}
            </div>
          ))}
        </div>
      )}

      {selectedUC && history.length > 0 && (
        <div className="card" style={{ overflow: "hidden" }}>
          <div style={{ padding: "16px 20px", borderBottom: "1px solid var(--border)" }}>
            <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: "var(--ink-2)" }}>Histórico de consumo — {selectedUC}</h3>
          </div>
          <table className="hist-table">
            <thead>
              <tr>
                <th>Referência</th>
                <th>Consumo</th>
                <th>Valor pago</th>
                <th>Bandeira</th>
              </tr>
            </thead>
            <tbody>
              {history.map((r) => (
                <tr key={r.referencia}>
                  <td className="date">{r.referencia}</td>
                  <td className="num">{fmtNum(r.consumo_kwh, 0)} kWh</td>
                  <td className="num" style={{ color: "var(--ink-2)" }}>{r.total_pagar_brl !== null ? fmtBRL(r.total_pagar_brl) : "--"}</td>
                  <td className="num">{r.bandeira ?? "--"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
