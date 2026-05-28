// Shapes returned by the public /api/events endpoints. Mirror backend DTOs.

import type {EventDTO, ProgramEntryDTO} from './types';

export interface PublicEventDetail {
  event: EventDTO;
  program: ProgramEntryDTO[];
  donation_config: {suggested_minor: number; min_minor: number} | null;
  phases: Phase[];
  categories: Category[];
  durations: Duration[];
  prices: Price[];
  capacity_full: boolean;
}

export interface Phase {
  id: string;
  name: string;
  starts_at: string;
  ends_at: string;
  ordering: number;
}

export interface Category {
  id: string;
  name: string;
  ordering: number;
}

export interface Duration {
  id: string;
  name: string;
  ordering: number;
}

export interface Price {
  id: string;
  phase_id: string;
  category_id: string;
  duration_id: string | null;
  amount_minor: number;
}

export interface QuoteResponse {
  currency: string;
  line_items: QuoteLineItem[];
  subtotal_minor: number;
  discount_minor: number;
  total_minor: number;
  applied_coupon_code?: string;
  applied_coupon_id?: string;
  phase?: {id: string; name: string};
}

export interface QuoteLineItem {
  description: string;
  amount_minor: number;
  phase_id?: string;
  category_id?: string;
  duration_id?: string;
}

export interface BookingResponse {
  outcome: 'booked' | 'waitlisted';

  // outcome=booked
  booking_id?: string;
  booking_reference?: string;
  status?: string;
  total_amount_minor?: number;
  currency?: string;
  participant_count?: number;
  payment_method?: string;
  reservation_expires_at?: string;

  // outcome=waitlisted
  waitlist_id?: string;
  waitlist_position?: number;
  waitlist_total?: number;
}

export function formatMoney(minor: number, currency: string): string {
  const whole = Math.floor(minor / 100);
  const cents = Math.abs(minor % 100);
  const cs = cents.toString().padStart(2, '0');
  return `${whole},${cs} ${currency}`;
}
