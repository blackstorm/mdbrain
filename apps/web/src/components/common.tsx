/** @jsxImportSource ../../../../packages/ui/src */
import { raw } from "../../../../packages/ui/src/index";
import { renderLucideIcon } from "../icons";

interface IconProps {
  name: string;
  className?: string;
}

interface AlertProps {
  message: string;
  kind?: "error" | "success" | "info";
}

interface FormFieldProps {
  id: string;
  name: string;
  label: string;
  type?: string;
  placeholder?: string;
  hint?: string;
  required?: boolean;
  autocomplete?: string;
  value?: string;
  minlength?: number;
  autofocus?: boolean;
}

interface EmptyStateProps {
  icon: string;
  title: string;
  message: string;
  ctaText?: string;
  ctaAction?: string;
  ctaIcon?: string;
  gridFull?: boolean;
}

export function Icon({ name, className = "" }: IconProps) {
  return raw(renderLucideIcon(name, className));
}

export function Alert({ message, kind = "error" }: AlertProps) {
  const iconName =
    kind === "success" ? "circle-check" : kind === "info" ? "info" : "circle-alert";

  return (
    <div class={`alert alert-${kind}`}>
      <Icon name={iconName} className="icon-sm" />
      <span>{message}</span>
    </div>
  );
}

export function FormField({
  id,
  name,
  label,
  type = "text",
  placeholder,
  hint,
  required = false,
  autocomplete,
  value,
  minlength,
  autofocus = false,
}: FormFieldProps) {
  return (
    <div class="form-field">
      <label class="form-label" for={id}>
        {label}
      </label>
      <input
        type={type}
        id={id}
        name={name}
        class="form-input"
        placeholder={placeholder}
        autocomplete={autocomplete}
        value={value}
        minlength={minlength}
        required={required}
        autofocus={autofocus}
      />
      {hint ? <span class="form-hint">{hint}</span> : null}
    </div>
  );
}

export function EmptyState({
  icon,
  title,
  message,
  ctaText,
  ctaAction,
  ctaIcon,
  gridFull = false,
}: EmptyStateProps) {
  const content = (
    <div class="empty-state">
      <div class="empty-icon">
        <Icon name={icon} className="w-8 h-8 text-[var(--text-tertiary)]" />
      </div>
      <h3>{title}</h3>
      <p>{message}</p>
      {ctaText ? (
        <button onclick={ctaAction} class="btn btn-primary">
          {ctaIcon ? <Icon name={ctaIcon} className="icon-sm" /> : null}
          <span>{ctaText}</span>
        </button>
      ) : null}
    </div>
  );

  return gridFull ? <div class="col-span-full">{content}</div> : content;
}
