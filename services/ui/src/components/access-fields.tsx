import { useTranslation } from "react-i18next"

import { Checkbox } from "@/components/ui/checkbox"
import {
	Field,
	FieldDescription,
	FieldError,
	FieldLabel,
	FieldLegend,
	FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { Permission, Role } from "@/graphql/generated/enums"
import { PERMISSIONS, ROLES } from "@/lib/access"

export interface Access {
	roles: Role[]
	permissions: Permission[]
	rank: number
}

export interface AccessErrors {
	roles?: string
	permissions?: string
	rank?: string
}

// AccessFields edits what a user may do (roles, and permissions beyond them)
// and who they may do it to (rank).
export function AccessFields({
	value,
	onChange,
	maxRank,
	errors,
}: {
	value: Access
	onChange: (value: Access) => void
	// The highest rank the viewer may hand out.
	maxRank: number
	errors?: AccessErrors
}) {
	const { t } = useTranslation()

	const toggle = <T,>(list: T[], item: T, on: boolean) =>
		on ? [...list, item] : list.filter((other) => other !== item)

	return (
		<>
			<FieldSet>
				<FieldLegend variant="label">{t("userForm.roles")}</FieldLegend>
				{ROLES.map((role) => (
					<Field
						key={role}
						orientation="horizontal"
					>
						<Checkbox
							id={`role-${role}`}
							checked={value.roles.includes(role)}
							onCheckedChange={(checked) =>
								onChange({
									...value,
									roles: toggle(value.roles, role, checked === true),
								})
							}
						/>
						<div className="flex flex-col gap-0.5">
							<FieldLabel htmlFor={`role-${role}`}>
								{t(`roles.${role}`)}
							</FieldLabel>
							<FieldDescription>
								{t(`roleDescriptions.${role}`)}
							</FieldDescription>
						</div>
					</Field>
				))}
				<FieldError>{errors?.roles}</FieldError>
			</FieldSet>
			<FieldSet>
				<FieldLegend variant="label">{t("userForm.permissions")}</FieldLegend>
				<FieldDescription>{t("userForm.permissionsHint")}</FieldDescription>
				{PERMISSIONS.map((permission) => (
					<Field
						key={permission}
						orientation="horizontal"
					>
						<Checkbox
							id={`permission-${permission}`}
							checked={value.permissions.includes(permission)}
							onCheckedChange={(checked) =>
								onChange({
									...value,
									permissions: toggle(
										value.permissions,
										permission,
										checked === true,
									),
								})
							}
						/>
						<FieldLabel htmlFor={`permission-${permission}`}>
							{t(`permissions.${permission}`)}
						</FieldLabel>
					</Field>
				))}
				<FieldError>{errors?.permissions}</FieldError>
			</FieldSet>
			<Field data-invalid={errors?.rank !== undefined}>
				<FieldLabel htmlFor="rank">{t("userForm.rank")}</FieldLabel>
				<Input
					id="rank"
					type="number"
					inputMode="numeric"
					min={0}
					max={maxRank}
					step={1}
					// An emptied field is NaN until something is typed; the form
					// won't submit it, since the field is required.
					value={Number.isNaN(value.rank) ? "" : value.rank}
					aria-invalid={errors?.rank !== undefined}
					onChange={(event) =>
						onChange({ ...value, rank: event.target.valueAsNumber })
					}
					required
				/>
				{errors?.rank ? (
					<FieldError>{errors.rank}</FieldError>
				) : (
					<FieldDescription>
						{t("userForm.rankHint", { max: maxRank })}
					</FieldDescription>
				)}
			</Field>
		</>
	)
}
