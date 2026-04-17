"use client";

import { useState } from "react";
import {
  useModelFamilies,
  useDeleteFamily,
  useDeleteMode,
  useUpdateMode,
} from "@/lib/hooks";
import type {
  FamilyWithModes,
  ModelMode,
} from "@/lib/hooks/use-model-families";
import {
  Button,
  Skeleton,
  Alert,
  AlertDescription,
  Badge,
  ConfirmProvider,
  useConfirm,
} from "@/components/ui";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui";
import { Switch } from "@/components/ui";
import { useToast } from "@/components/ui/toaster";
import { useTranslation } from "@/lib/i18n/context";
import { getAPIErrorMessage } from "@/lib/api-client";
import { AlertCircle, Plus, Pencil, Trash2 } from "lucide-react";
import { FamilyDialog } from "./family-dialog";
import { ModeDialog } from "./mode-dialog";

function ModelsPageInner() {
  const { t } = useTranslation();
  const { toast } = useToast();
  const confirm = useConfirm();
  const { data: families, isLoading, error } = useModelFamilies();
  const deleteFamily = useDeleteFamily();
  const deleteMode = useDeleteMode();
  const updateMode = useUpdateMode();

  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [familyDialog, setFamilyDialog] = useState<{
    open: boolean;
    family?: FamilyWithModes;
  }>({ open: false });
  const [modeDialog, setModeDialog] = useState<{
    open: boolean;
    familyId: number;
    familyType: string;
    mode?: ModelMode;
  }>({
    open: false,
    familyId: 0,
    familyType: "",
  });

  const selected = families?.find((f) => f.id === selectedId) ?? null;

  const showMutationError = (error: unknown) => {
    toast({
      title: t.common.error,
      description: getAPIErrorMessage(error, t.common.operationFailed),
      variant: "destructive",
    });
  };

  const handleDeleteFamily = async (f: FamilyWithModes) => {
    const msg = t.models.deleteFamilyConfirm.replace("{name}", f.model);
    if (!(await confirm({ title: msg, variant: "destructive" }))) return;
    try {
      await deleteFamily.mutateAsync(f.id);
      if (selectedId === f.id) setSelectedId(null);
      toast({ title: t.common.success, description: t.models.deleteSuccess });
    } catch (error) {
      showMutationError(error);
    }
  };

  const handleDeleteMode = async (m: ModelMode) => {
    const msg = t.models.deleteModeConfirm.replace("{name}", m.mode);
    if (!(await confirm({ title: msg, variant: "destructive" }))) return;
    try {
      await deleteMode.mutateAsync(m.id);
      toast({ title: t.common.success, description: t.models.deleteSuccess });
    } catch (error) {
      showMutationError(error);
    }
  };

  const handleToggleMode = async (m: ModelMode) => {
    try {
      await updateMode.mutateAsync({
        id: m.id,
        data: {
          model_id: m.model_id,
          mode: m.mode,
          enabled: !m.enabled,
          pool_floor_override: m.pool_floor_override,
          upstream_model: m.upstream_model,
          upstream_mode: m.upstream_mode,
          quota_override: m.quota_override,
        },
      });
      toast({ title: t.common.success, description: t.models.updateSuccess });
    } catch (error) {
      showMutationError(error);
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-8 max-w-6xl">
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-semibold tracking-tight">
            {t.models.title}
          </h1>
          <p className="text-muted text-sm">{t.models.description}</p>
        </div>
        <div className="rounded-md border border-[rgba(0,0,0,0.06)] shadow-sm bg-surface p-4 space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-8 max-w-6xl">
        <h1 className="text-3xl font-semibold tracking-tight">
          {t.models.title}
        </h1>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t.models.loadError}</AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-semibold tracking-tight">
            {t.models.title}
          </h1>
          <p className="text-muted text-sm">{t.models.description}</p>
        </div>
        <Button onClick={() => setFamilyDialog({ open: true })}>
          <Plus className="h-4 w-4 mr-1" />
          {t.models.addFamily}
        </Button>
      </div>

      {!families || families.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-base font-medium">{t.models.noFamilies}</p>
          <p className="text-sm text-muted mt-1">{t.models.noFamiliesDesc}</p>
          <Button
            className="mt-4"
            onClick={() => setFamilyDialog({ open: true })}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t.models.addFamily}
          </Button>
        </div>
      ) : (
        <div className="flex flex-col md:flex-row gap-4">
          {/* Left panel — family list */}
          <div className="w-full md:w-[280px] shrink-0 rounded-md border border-[rgba(0,0,0,0.06)] bg-surface overflow-y-auto max-h-[70vh]">
            {families.map((f) => (
              <div
                key={f.id}
                onClick={() => setSelectedId(f.id)}
                className={`relative flex items-center justify-between px-3 py-2.5 cursor-pointer transition-colors text-sm ${
                  selectedId === f.id
                    ? "bg-[rgba(0,95,184,0.08)]"
                    : "hover:bg-[rgba(0,0,0,0.03)]"
                }`}
              >
                {selectedId === f.id && (
                  <div className="absolute left-0 top-[20%] bottom-[20%] w-[3px] bg-[#005FB8] rounded-full" />
                )}
                <div className="flex items-center gap-2 min-w-0">
                  <span className="truncate font-medium">{f.model}</span>
                  <Badge variant="outline" className="text-[11px] shrink-0">
                    {f.type}
                  </Badge>
                  {!f.enabled && (
                    <Badge
                      variant="secondary"
                      className="text-[11px] shrink-0 opacity-60"
                    >
                      {t.common.disabled}
                    </Badge>
                  )}
                </div>
                <div className="flex items-center gap-1 shrink-0 ml-2">
                  <button
                    type="button"
                    aria-label={`${t.models.editFamily} ${f.model}`}
                    className="p-1 rounded hover:bg-[rgba(0,0,0,0.06)]"
                    onClick={(e) => {
                      e.stopPropagation();
                      setFamilyDialog({ open: true, family: f });
                    }}
                  >
                    <Pencil className="h-3.5 w-3.5 text-muted" />
                  </button>
                  <button
                    type="button"
                    aria-label={`${t.models.deleteFamily} ${f.model}`}
                    className="p-1 rounded hover:bg-[rgba(196,43,28,0.08)]"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDeleteFamily(f);
                    }}
                  >
                    <Trash2 className="h-3.5 w-3.5 text-destructive" />
                  </button>
                </div>
              </div>
            ))}
          </div>

          {/* Right panel — modes */}
          <div className="flex-1 min-w-0">
            {!selected ? (
              <div className="flex flex-col items-center justify-center py-16 text-center">
                <p className="text-base font-medium">{t.models.selectFamily}</p>
                <p className="text-sm text-muted mt-1">
                  {t.models.selectFamilyDesc}
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <h2 className="text-lg font-semibold">{selected.model}</h2>
                  <Button
                    size="sm"
                    onClick={() =>
                      setModeDialog({
                        open: true,
                        familyId: selected.id,
                        familyType: selected.type,
                      })
                    }
                  >
                    <Plus className="h-4 w-4 mr-1" />
                    {t.models.addMode}
                  </Button>
                </div>
                {selected.modes.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-12 text-center">
                    <p className="text-base font-medium">{t.models.noModes}</p>
                    <p className="text-sm text-muted mt-1">
                      {t.models.noModesDesc}
                    </p>
                  </div>
                ) : (
                  <div className="rounded-md border border-[rgba(0,0,0,0.06)] overflow-hidden">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Mode</TableHead>
                          <TableHead>Upstream Model</TableHead>
                          <TableHead>Upstream Mode</TableHead>
                          <TableHead>Pool Floor</TableHead>
                          <TableHead>Enabled</TableHead>
                          <TableHead className="w-[80px]">Actions</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {selected.modes.map((m) => (
                          <TableRow
                            key={m.id}
                            className={!m.enabled ? "opacity-60" : ""}
                          >
                            <TableCell className="font-medium">
                              {m.mode}
                            </TableCell>
                            <TableCell>{m.upstream_model || "-"}</TableCell>
                            <TableCell>{m.upstream_mode || "-"}</TableCell>
                            <TableCell>{m.pool_floor_override || "—"}</TableCell>
                            <TableCell>
                              <Switch
                                checked={m.enabled}
                                onCheckedChange={() => handleToggleMode(m)}
                              />
                            </TableCell>
                            <TableCell>
                              <div className="flex items-center gap-1">
                                <button
                                  type="button"
                                  aria-label={`${t.models.editMode} ${m.mode}`}
                                  className="p-1 rounded hover:bg-[rgba(0,0,0,0.06)]"
                                  onClick={() =>
                                    setModeDialog({
                                      open: true,
                                      familyId: selected.id,
                                      familyType: selected.type,
                                      mode: m,
                                    })
                                  }
                                >
                                  <Pencil className="h-3.5 w-3.5 text-muted" />
                                </button>
                                <button
                                  type="button"
                                  aria-label={`${t.models.deleteMode} ${m.mode}`}
                                  className="p-1 rounded hover:bg-[rgba(196,43,28,0.08)]"
                                  onClick={() => handleDeleteMode(m)}
                                >
                                  <Trash2 className="h-3.5 w-3.5 text-destructive" />
                                </button>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      <FamilyDialog
        open={familyDialog.open}
        onOpenChange={(open) => !open && setFamilyDialog({ open: false })}
        family={familyDialog.family}
      />
      <ModeDialog
        open={modeDialog.open}
        onOpenChange={(open) =>
          !open && setModeDialog({ open: false, familyId: 0, familyType: "" })
        }
        familyId={modeDialog.familyId}
        familyType={modeDialog.familyType}
        mode={modeDialog.mode}
      />
    </div>
  );
}

export function ModelsPage() {
  return (
    <ConfirmProvider>
      <ModelsPageInner />
    </ConfirmProvider>
  );
}
