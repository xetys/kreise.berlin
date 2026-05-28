// Shared shapes returned by the backend API. Mirror the Go DTOs.

export interface EventDTO {
  id: string;
  slug: string;
  name: string;
  description: string;
  banner_ref: string;
  banner_url?: string;
  color_primary: string;
  color_secondary: string;
  color_text: string;
  location: string;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
  participant_limit: number | null;
  pricing_mode: 'matrix' | 'donation';
  currency: string;
  default_locale: 'de' | 'en';
  is_public: boolean;
  is_archived: boolean;
  created_by: string;
  created_at: string;
  payment_timing: 'beforehand' | 'at_door';
  bank_iban?: string;
  bank_bic?: string;
  bank_account_holder?: string;
  paypal_handle?: string;
  payment_test_mode: boolean;
}

export interface EventListResponse {
  events: EventDTO[];
}

export interface ProgramEntryDTO {
  id: string;
  event_id: string;
  starts_at: string;
  ends_at: string | null;
  title: string;
  description: string;
  ordering: number;
}
